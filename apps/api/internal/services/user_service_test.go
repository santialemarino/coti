package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// User administration is all policy — who may do what to whom — so it is tested against
// in-memory fakes. The tenant boundary itself is proven against a real database in the
// repository integration tests.

var (
	otherUserID    = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	assignedBranch = uuid.MustParse("44444444-4444-4444-8444-444444444444")
)

// fakeAdminUsers records what the service asked of app_user, so a test can assert the
// account it was scoped to and whether the epoch was bumped.
type fakeAdminUsers struct {
	stored        map[uuid.UUID]*domain.AppUser
	createdIn     []uuid.UUID
	createdHash   string
	updated       []domain.UserUpdate
	deactivated   []uuid.UUID
	epochBumpedID []uuid.UUID
	verified      []uuid.UUID
}

func newFakeAdminUsers(users ...*domain.AppUser) *fakeAdminUsers {
	stored := make(map[uuid.UUID]*domain.AppUser, len(users))
	for _, u := range users {
		stored[u.ID] = u
	}
	return &fakeAdminUsers{stored: stored}
}

func (f *fakeAdminUsers) List(_ context.Context, _ repository.Querier, _ uuid.UUID) ([]domain.AppUser, error) {
	out := make([]domain.AppUser, 0, len(f.stored))
	for _, u := range f.stored {
		out = append(out, *u)
	}
	return out, nil
}

func (f *fakeAdminUsers) GetByID(_ context.Context, _ repository.Querier, _, id uuid.UUID) (*domain.AppUser, error) {
	u, ok := f.stored[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *u
	return &copied, nil
}

func (f *fakeAdminUsers) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewUser, passwordHash string,
) (*domain.AppUser, error) {
	f.createdIn = append(f.createdIn, accountID)
	f.createdHash = passwordHash
	created := &domain.AppUser{
		ID: uuid.New(), AccountID: accountID, Name: in.Name, Email: in.Email,
		PasswordHash: passwordHash, Role: in.Role, IsActive: true, SessionEpoch: 1,
	}
	f.stored[created.ID] = created
	return created, nil
}

func (f *fakeAdminUsers) Update(
	_ context.Context, _ repository.Querier, accountID, id uuid.UUID, in domain.UserUpdate,
) (*domain.AppUser, error) {
	f.updated = append(f.updated, in)
	current, ok := f.stored[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	updated := *current
	updated.Name, updated.Email, updated.Role = in.Name, in.Email, in.Role
	if in.IsActive != nil {
		updated.IsActive = *in.IsActive
	}
	f.stored[id] = &updated
	return &updated, nil
}

func (f *fakeAdminUsers) Deactivate(_ context.Context, _ repository.Querier, _, id uuid.UUID) error {
	if _, ok := f.stored[id]; !ok {
		return domain.ErrNotFound
	}
	f.deactivated = append(f.deactivated, id)
	return nil
}

func (f *fakeAdminUsers) BumpSessionEpoch(_ context.Context, _ repository.Querier, _, id uuid.UUID) (int, error) {
	f.epochBumpedID = append(f.epochBumpedID, id)
	return 2, nil
}

func (f *fakeAdminUsers) MarkEmailVerified(_ context.Context, _ repository.Querier, _, id uuid.UUID) error {
	f.verified = append(f.verified, id)
	if u, ok := f.stored[id]; ok {
		at := time.Now()
		u.EmailVerifiedAt = &at
	}
	return nil
}

func (f *fakeAdminUsers) SellersForBranches(
	_ context.Context, _ repository.Querier, _ uuid.UUID, branchIDs []uuid.UUID,
) ([]domain.Seller, error) {
	if branchIDs != nil && len(branchIDs) == 0 {
		return nil, nil
	}
	var sellers []domain.Seller
	for _, u := range f.stored {
		if u.Role != domain.UserRoleSeller || !u.IsActive {
			continue
		}
		sellers = append(sellers, domain.Seller{ID: u.ID, Name: u.Name})
	}
	return sellers, nil
}

type fakeAssignments struct {
	byUser   map[uuid.UUID][]uuid.UUID
	replaced map[uuid.UUID][]uuid.UUID
}

func newFakeAssignments() *fakeAssignments {
	return &fakeAssignments{
		byUser:   map[uuid.UUID][]uuid.UUID{},
		replaced: map[uuid.UUID][]uuid.UUID{},
	}
}

func (f *fakeAssignments) ListByUsers(
	_ context.Context, _ repository.Querier, _ uuid.UUID, userIDs []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	for _, id := range userIDs {
		if branches, ok := f.byUser[id]; ok {
			out[id] = branches
		}
	}
	return out, nil
}

func (f *fakeAssignments) Replace(
	_ context.Context, _ repository.Querier, _, userID uuid.UUID, branchIDs []uuid.UUID,
) error {
	f.replaced[userID] = branchIDs
	f.byUser[userID] = branchIDs
	return nil
}

// fakeBranchExistence answers whether branch ids belong to the account. known is what
// ExistAllInAccount accepts; anything else is another account's branch.
type fakeBranchExistence struct {
	known []uuid.UUID
}

func (f *fakeBranchExistence) ExistAllInAccount(
	_ context.Context, _ repository.Querier, _ uuid.UUID, ids []uuid.UUID,
) (bool, error) {
	for _, id := range ids {
		found := false
		for _, k := range f.known {
			if k == id {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

// fakeUserLinks is the link surface plus the latest invite per user, which a test seeds to
// put a user in a given invite state. redeemedMeanwhile and resentMeanwhile stand for a redeem or
// another resend that committed between the read of the latest invite and the retiring of it.
type fakeUserLinks struct {
	fakeAuthTokens
	latest            map[uuid.UUID]domain.AuthToken
	redeemedMeanwhile bool
	resentMeanwhile   bool
}

func (f *fakeUserLinks) InvalidateActive(
	ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, tokenType domain.AuthTokenType,
) (int64, error) {
	if _, err := f.fakeAuthTokens.InvalidateActive(ctx, q, accountID, userID, tokenType); err != nil {
		return 0, err
	}
	latest, ok := f.latest[userID]
	switch {
	case !ok || latest.Type != tokenType || latest.ConsumedAt != nil:
		return 0, nil
	case f.redeemedMeanwhile:
		at := fixedNow
		latest.ConsumedAt = &at
		f.latest[userID] = latest
		return 0, nil
	case f.resentMeanwhile:
		f.latest[userID] = outstandingInvite(userID, fixedNow.Add(testInviteTTL))
		return 0, nil
	}
	return 1, nil
}

func (f *fakeUserLinks) LatestInvitesByUsers(
	_ context.Context, _ repository.Querier, _ uuid.UUID, userIDs []uuid.UUID,
) (map[uuid.UUID]domain.AuthToken, error) {
	out := map[uuid.UUID]domain.AuthToken{}
	for _, id := range userIDs {
		if t, ok := f.latest[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

// fakeAddressVerifier records what an address change mailed to each mailbox.
type fakeAddressVerifier struct {
	verificationsTo []string
	noticesTo       []string
}

func (f *fakeAddressVerifier) SendForNewAddress(_ context.Context, user domain.AppUser) error {
	f.verificationsTo = append(f.verificationsTo, user.Email)
	return nil
}

func (f *fakeAddressVerifier) NotifyAddressChangedByAdmin(_ context.Context, previous domain.AppUser, _ string) {
	f.noticesTo = append(f.noticesTo, previous.Email)
}

const testInviteTTL = 168 * time.Hour

type userHarness struct {
	svc         *UserService
	db          *fakeDB
	users       *fakeAdminUsers
	assignments *fakeAssignments
	branches    *fakeBranchExistence
	links       *fakeUserLinks
	mail        *fakeMail
	verifier    *fakeAddressVerifier
}

func newUserHarness(stored ...*domain.AppUser) *userHarness {
	return newUserHarnessDelivering(true, stored...)
}

func newUserHarnessDelivering(mailDelivers bool, stored ...*domain.AppUser) *userHarness {
	h := &userHarness{
		db:          &fakeDB{},
		users:       newFakeAdminUsers(stored...),
		assignments: newFakeAssignments(),
		branches:    &fakeBranchExistence{known: []uuid.UUID{assignedBranch}},
		links:       &fakeUserLinks{latest: map[uuid.UUID]domain.AuthToken{}},
		mail:        &fakeMail{},
		verifier:    &fakeAddressVerifier{},
	}
	cfg := testAuthConfig()
	cfg.InviteTTL = testInviteTTL
	h.svc = NewUserService(h.db, h.users, h.assignments, h.branches, h.links, h.mail, h.verifier,
		slog.New(slog.NewTextHandler(io.Discard, nil)), cfg,
		config.WebConfig{BackofficeURL: "https://backoffice.example"}, mailDelivers,
		func() time.Time { return fixedNow })
	h.svc.invites.newSecret = func() (string, error) { return testRawResetToken, nil }
	return h
}

func adminTenant() domain.Tenant {
	return domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin}
}

func storedAdmin() *domain.AppUser {
	return &domain.AppUser{
		ID: testUserID, AccountID: testAccountID, Name: "Admin", Email: "admin@corralon.test",
		Role: domain.UserRoleAdmin, IsActive: true, SessionEpoch: 1,
	}
}

func storedSeller() *domain.AppUser {
	return &domain.AppUser{
		ID: otherUserID, AccountID: testAccountID, Name: "Vendedor", Email: "v@corralon.test",
		Role: domain.UserRoleSeller, IsActive: true, SessionEpoch: 1,
	}
}

func validNewUser() domain.NewUser {
	return domain.NewUser{
		Name: "Nuevo Vendedor", Email: "nuevo@corralon.test", Password: "Una-clave-larga1",
		Role: domain.UserRoleSeller, BranchIDs: []uuid.UUID{assignedBranch},
	}
}

// The account is the caller's, always. A body carrying another account is not a case the
// service has to reject, because it never reads one — this asserts that stays true.
func TestUserService_CreateUsesTheTenantAccount(t *testing.T) {
	h := newUserHarness(storedAdmin())

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), validNewUser())
	if err != nil {
		t.Fatalf("CreateUser() = %v, want no error", err)
	}
	if created.AccountID != testAccountID {
		t.Errorf("created in account %v, want %v", created.AccountID, testAccountID)
	}
	if len(h.users.createdIn) != 1 || h.users.createdIn[0] != testAccountID {
		t.Errorf("repository scoped to %v, want [%v]", h.users.createdIn, testAccountID)
	}
	if len(h.db.scopes) != 1 || h.db.scopes[0] != testAccountID {
		t.Errorf("transaction scoped to %v, want [%v]", h.db.scopes, testAccountID)
	}
}

// A user handed a password is trusted on the admin's word, in the same transaction that created
// them. Nothing else ever would: no path mails these users a confirmation link, so without this
// they carry a null email_verified_at forever and AUTH_REQUIRE_VERIFIED_EMAIL locks them out of
// an account they were deliberately given access to.
func TestUserService_CreateMarksTheAddressVerified(t *testing.T) {
	h := newUserHarness(storedAdmin())

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), validNewUser())
	if err != nil {
		t.Fatalf("CreateUser() = %v, want no error", err)
	}
	if len(h.users.verified) != 1 || h.users.verified[0] != created.ID {
		t.Fatalf("verified %v, want [%v]", h.users.verified, created.ID)
	}
	if stored := h.users.stored[created.ID]; stored.EmailVerifiedAt == nil {
		t.Error("the created user carries a null email_verified_at")
	}
	// One transaction for the whole creation: a verification written outside it could survive a
	// rollback that took the user with it.
	if len(h.db.scopes) != 1 {
		t.Errorf("creation opened %d transactions, want 1", len(h.db.scopes))
	}
}

// The password must reach the database hashed, and the response must never carry either the
// plaintext or the hash.
func TestUserService_CreateHashesThePassword(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if err != nil {
		t.Fatalf("CreateUser() = %v, want no error", err)
	}
	if h.users.createdHash == in.Password {
		t.Fatal("the plaintext password was stored")
	}
	if bcrypt.CompareHashAndPassword([]byte(h.users.createdHash), []byte(in.Password)) != nil {
		t.Error("the stored hash does not verify against the password")
	}
	if created.PasswordHash != h.users.createdHash {
		t.Error("the domain user should carry the hash the repository stored")
	}
}

func TestUserService_CreateRejectsAShortPassword(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()
	in.Password = "corta"

	_, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("CreateUser() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.users.createdIn) != 0 {
		t.Error("a rejected password must not reach the repository")
	}
}

func TestUserService_CreateRejectsARoleOutsideTheEnum(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()
	in.Role = "OWNER"

	_, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("CreateUser() = %v, want %v", err, domain.ErrInvalidInput)
	}
}

// A foreign key does not confine a child row to its account, so a branch id from another
// account has to be refused before user_branch is written.
func TestUserService_CreateRejectsABranchFromAnotherAccount(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()
	in.BranchIDs = []uuid.UUID{uuid.New()}

	_, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateUser() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.users.createdIn) != 0 {
		t.Error("the user must not be created when a branch id is refused")
	}
}

func TestUserService_CreateAssignsBranchesAndDedupes(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()
	in.BranchIDs = []uuid.UUID{assignedBranch, assignedBranch}

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if err != nil {
		t.Fatalf("CreateUser() = %v, want no error", err)
	}
	want := []uuid.UUID{assignedBranch}
	if !reflect.DeepEqual(h.assignments.replaced[created.ID], want) {
		t.Errorf("assigned %v, want %v", h.assignments.replaced[created.ID], want)
	}
	if !reflect.DeepEqual(created.BranchIDs, want) {
		t.Errorf("response BranchIDs = %v, want %v", created.BranchIDs, want)
	}
}

func TestUserService_CreateNormalizesTheEmail(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := validNewUser()
	in.Email = "  Nuevo@Corralon.TEST "

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if err != nil {
		t.Fatalf("CreateUser() = %v, want no error", err)
	}
	if created.Email != "nuevo@corralon.test" {
		t.Errorf("Email = %q, want %q", created.Email, "nuevo@corralon.test")
	}
}

// An admin locking themselves out has no recovery path: there is no account-level reset and
// no invitation flow, so the guard is the only thing standing between a misclick and a dead
// account.
func TestUserService_AnAdminCannotDeactivateThemselves(t *testing.T) {
	h := newUserHarness(storedAdmin())

	err := h.svc.DeactivateUser(context.Background(), adminTenant(), testUserID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("DeactivateUser(self) = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.users.deactivated) != 0 {
		t.Error("self-deactivation reached the repository")
	}
}

func TestUserService_AnAdminCannotDeactivateThemselvesThroughUpdate(t *testing.T) {
	h := newUserHarness(storedAdmin())
	inactive := false

	_, err := h.svc.UpdateUser(context.Background(), adminTenant(), testUserID, domain.UserUpdate{
		Name: "Admin", Email: "admin@corralon.test", Role: domain.UserRoleAdmin, IsActive: &inactive,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("UpdateUser(self, is_active=false) = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.users.updated) != 0 {
		t.Error("self-deactivation reached the repository")
	}
}

// Self-demotion is the other way to lock yourself out of the admin functions.
func TestUserService_AnAdminCannotChangeTheirOwnRole(t *testing.T) {
	h := newUserHarness(storedAdmin())

	_, err := h.svc.UpdateUser(context.Background(), adminTenant(), testUserID, domain.UserUpdate{
		Name: "Admin", Email: "admin@corralon.test", Role: domain.UserRoleSeller,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("UpdateUser(self, role=SELLER) = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.users.updated) != 0 {
		t.Error("self-demotion reached the repository")
	}
}

// Their own address goes through the self-service change, which re-checks the password; an admin
// session left open must not be enough to move the account's way back in to another mailbox.
func TestUserService_AnAdminCannotChangeTheirOwnEmail(t *testing.T) {
	h := newUserHarness(storedAdmin())

	_, err := h.svc.UpdateUser(context.Background(), adminTenant(), testUserID, domain.UserUpdate{
		Name: "Admin", Email: "otra@corralon.test", Role: domain.UserRoleAdmin,
	})
	if !errors.Is(err, domain.ErrInvalidInput) || domain.CodeOf(err) != domain.CodeSelfEmailChange {
		t.Fatalf("UpdateUser(self, new email) = %v (%s), want %s", err, domain.CodeOf(err),
			domain.CodeSelfEmailChange)
	}
	if len(h.users.updated) != 0 || len(h.links.invalidatedAll) != 0 {
		t.Error("the self email change reached the repository")
	}
}

// Editing your own profile stays allowed — only the role, the address and the active flag are
// guarded.
func TestUserService_AnAdminMayEditTheirOwnProfile(t *testing.T) {
	h := newUserHarness(storedAdmin())

	updated, err := h.svc.UpdateUser(context.Background(), adminTenant(), testUserID, domain.UserUpdate{
		Name: "Nombre Nuevo", Email: "admin@corralon.test", Role: domain.UserRoleAdmin,
	})
	if err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if updated.Name != "Nombre Nuevo" {
		t.Errorf("Name = %q, want %q", updated.Name, "Nombre Nuevo")
	}
}

// Without the epoch bump a deactivated user keeps working until their access token expires.
func TestUserService_DeactivateBumpsTheSessionEpoch(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())

	if err := h.svc.DeactivateUser(context.Background(), adminTenant(), otherUserID); err != nil {
		t.Fatalf("DeactivateUser() = %v, want no error", err)
	}
	if !reflect.DeepEqual(h.users.epochBumpedID, []uuid.UUID{otherUserID}) {
		t.Errorf("epoch bumped for %v, want [%v]", h.users.epochBumpedID, otherUserID)
	}
}

func TestUserService_UpdateToInactiveBumpsTheSessionEpoch(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())
	inactive := false

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID, domain.UserUpdate{
		Name: "Vendedor", Email: "v@corralon.test", Role: domain.UserRoleSeller, IsActive: &inactive,
	}); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if !reflect.DeepEqual(h.users.epochBumpedID, []uuid.UUID{otherUserID}) {
		t.Errorf("epoch bumped for %v, want [%v]", h.users.epochBumpedID, otherUserID)
	}
}

// An edit that leaves the user active must not revoke their tokens, or every profile change
// would log the person out.
func TestUserService_UpdateThatKeepsTheUserActiveDoesNotBumpTheEpoch(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID, domain.UserUpdate{
		Name: "Otro Nombre", Email: "v@corralon.test", Role: domain.UserRoleSeller,
	}); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.users.epochBumpedID) != 0 {
		t.Errorf("epoch bumped %v times on an active-user edit, want 0", len(h.users.epochBumpedID))
	}
}

// A user of another account is invisible under row level security, so the service sees
// ErrNotFound rather than a forbidden row.
func TestUserService_DeactivateAnUnknownUserIsNotFound(t *testing.T) {
	h := newUserHarness(storedAdmin())

	err := h.svc.DeactivateUser(context.Background(), adminTenant(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("DeactivateUser() = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestUserService_ListReturnsBranchAssignments(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())
	h.assignments.byUser[otherUserID] = []uuid.UUID{assignedBranch}

	users, err := h.svc.ListUsers(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("ListUsers() = %v, want no error", err)
	}
	if len(users) != 2 {
		t.Fatalf("ListUsers() returned %d users, want 2", len(users))
	}
	for _, u := range users {
		if u.BranchIDs == nil {
			t.Errorf("user %s carries a nil branch list, want an empty one", u.Email)
		}
		if u.ID == otherUserID && !reflect.DeepEqual(u.BranchIDs, []uuid.UUID{assignedBranch}) {
			t.Errorf("seller assignments = %v, want [%v]", u.BranchIDs, assignedBranch)
		}
	}
}

// The picklist only offers sellers who could receive a new order: active sellers, and no
// admins or deactivated accounts, whatever role they carry.
func TestUserService_ListSellersSkipsAdminsAndDeactivated(t *testing.T) {
	gone := &domain.AppUser{
		ID: uuid.New(), AccountID: testAccountID, Name: "Gone", Email: "gone@corralon.test",
		Role: domain.UserRoleSeller, IsActive: false, SessionEpoch: 1,
	}
	h := newUserHarness(storedAdmin(), storedSeller(), gone)

	sellers, err := h.svc.ListSellers(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("ListSellers() = %v, want no error", err)
	}
	if len(sellers) != 1 {
		t.Fatalf("ListSellers() returned %d sellers, want 1", len(sellers))
	}
	if sellers[0].ID != otherUserID || sellers[0].Name != "Vendedor" {
		t.Errorf("seller = %+v, want the active seller", sellers[0])
	}
}

// A seller with no branch selection reads nothing rather than the whole account, so the
// picklist can never leak branches a caller does not reach.
func TestUserService_ListSellersSellersFailsClosedWithoutBranch(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleSeller}

	sellers, err := h.svc.ListSellers(context.Background(), tenant)
	if err != nil {
		t.Fatalf("ListSellers() = %v, want no error", err)
	}
	if len(sellers) != 0 {
		t.Errorf("ListSellers() returned %d sellers, want 0", len(sellers))
	}
}

func invitedNewUser() domain.NewUser {
	in := validNewUser()
	in.Password = ""
	in.Invite = true
	return in
}

// An invited user gets no password anyone knows and no confirmation yet: redeeming the link is
// what sets the first and proves the second.
func TestUserService_CreateWithInviteMailsALinkInsteadOfSettingAPassword(t *testing.T) {
	h := newUserHarness(storedAdmin())

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), invitedNewUser())
	if err != nil {
		t.Fatalf("CreateUser(invite) = %v, want no error", err)
	}
	if created.InviteStatus != domain.InviteStatusPending {
		t.Errorf("invite status = %q, want %q", created.InviteStatus, domain.InviteStatusPending)
	}
	if len(h.users.verified) != 0 {
		t.Error("an invited address was marked verified before anyone redeemed the link")
	}
	if bcrypt.CompareHashAndPassword([]byte(h.users.createdHash), []byte("")) == nil {
		t.Error("the invited user can log in with an empty password")
	}
	if len(h.links.created) != 1 {
		t.Fatalf("minted %d links, want 1", len(h.links.created))
	}
	token := h.links.created[0]
	if token.Type != domain.AuthTokenTypeInvite || token.UserID != created.ID {
		t.Errorf("minted a %s link for %v, want an invite for %v", token.Type, token.UserID, created.ID)
	}
	if !token.ExpiresAt.Equal(fixedNow.Add(testInviteTTL)) {
		t.Errorf("invite expires at %v, want %v", token.ExpiresAt, fixedNow.Add(testInviteTTL))
	}
	if token.TokenHash == testRawResetToken || token.TokenHash != hashToken(testRawResetToken) {
		t.Error("the invite is not stored as the hash of the mailed secret")
	}
	if len(h.mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(h.mail.sent))
	}
	sent := h.mail.sent[0]
	if sent.Event != domain.NotificationEventInvite || sent.To != created.Email {
		t.Errorf("mailed a %s to %s, want an INVITE to %s", sent.Event, sent.To, created.Email)
	}
	want := "https://backoffice.example/reset-password?token=" + testRawResetToken + "&invite=1"
	if sent.ActionURL != want {
		t.Errorf("link = %q, want %q", sent.ActionURL, want)
	}
}

func TestUserService_CreateWithInviteRefusesAPasswordToo(t *testing.T) {
	h := newUserHarness(storedAdmin())
	in := invitedNewUser()
	in.Password = "Una-clave-larga1"

	_, err := h.svc.CreateUser(context.Background(), adminTenant(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateUser(invite + password) = %v, want %v", err, domain.ErrInvalidInput)
	}
	if !strings.Contains(err.Error(), "chooses their own password") {
		t.Errorf("refused for %q, want the invite-and-password rule", err)
	}
	if len(h.users.createdIn) != 0 {
		t.Error("a user was created")
	}
}

// While mail only reaches the log nobody could redeem the link, so the user would exist with no
// way in at all.
func TestUserService_CreateWithInviteNeedsMailThatDelivers(t *testing.T) {
	h := newUserHarnessDelivering(false, storedAdmin())

	_, err := h.svc.CreateUser(context.Background(), adminTenant(), invitedNewUser())
	if !errors.Is(err, domain.ErrNotConfigured) || domain.CodeOf(err) != domain.CodeMailNotConfigured {
		t.Fatalf("CreateUser(invite) on console mail = %v (%s), want %s", err, domain.CodeOf(err),
			domain.CodeMailNotConfigured)
	}
	if len(h.users.createdIn) != 0 || len(h.mail.sent) != 0 {
		t.Error("a user was created or mailed")
	}
	// The password path is untouched by the transport.
	if _, err := h.svc.CreateUser(context.Background(), adminTenant(), validNewUser()); err != nil {
		t.Fatalf("CreateUser(password) on console mail = %v, want no error", err)
	}
}

func outstandingInvite(userID uuid.UUID, expiresAt time.Time) domain.AuthToken {
	return domain.AuthToken{
		ID: uuid.New(), AccountID: testAccountID, UserID: userID, Type: domain.AuthTokenTypeInvite,
		ExpiresAt: expiresAt, CreatedAt: expiresAt.Add(-testInviteTTL),
	}
}

func TestUserService_ListDerivesTheInviteStatus(t *testing.T) {
	pending, expired, redeemed := storedSeller(), storedSeller(), storedSeller()
	pending.ID, expired.ID, redeemed.ID = uuid.New(), uuid.New(), uuid.New()
	h := newUserHarness(storedAdmin(), pending, expired, redeemed)
	h.links.latest[pending.ID] = outstandingInvite(pending.ID, fixedNow.Add(time.Hour))
	h.links.latest[expired.ID] = outstandingInvite(expired.ID, fixedNow.Add(-time.Hour))
	used := outstandingInvite(redeemed.ID, fixedNow.Add(time.Hour))
	consumedAt := fixedNow.Add(-time.Minute)
	used.ConsumedAt = &consumedAt
	h.links.latest[redeemed.ID] = used

	users, err := h.svc.ListUsers(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("ListUsers() = %v, want no error", err)
	}
	want := map[uuid.UUID]domain.InviteStatus{
		testUserID:  domain.InviteStatusNone,
		pending.ID:  domain.InviteStatusPending,
		expired.ID:  domain.InviteStatusExpired,
		redeemed.ID: domain.InviteStatusNone,
	}
	for _, u := range users {
		if u.InviteStatus != want[u.ID] {
			t.Errorf("%v invite status = %q, want %q", u.ID, u.InviteStatus, want[u.ID])
		}
	}
}

func TestUserService_ResendInvite(t *testing.T) {
	t.Run("an expired invite is replaced and mailed", func(t *testing.T) {
		h := newUserHarness(storedAdmin(), storedSeller())
		h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(-time.Hour))

		if err := h.svc.ResendInvite(context.Background(), adminTenant(), otherUserID); err != nil {
			t.Fatalf("ResendInvite() = %v, want no error", err)
		}
		if len(h.links.invalidated) != 1 || h.links.invalidated[0].tokenType != domain.AuthTokenTypeInvite {
			t.Errorf("retired %v, want the previous invite", h.links.invalidated)
		}
		if len(h.links.created) != 1 || h.links.created[0].Type != domain.AuthTokenTypeInvite {
			t.Fatalf("minted %v, want one invite", h.links.created)
		}
		if len(h.mail.sent) != 1 || h.mail.sent[0].Event != domain.NotificationEventInvite {
			t.Fatalf("mailed %v, want one invite", h.mail.sent)
		}
	})

	refusals := []struct {
		name   string
		setup  func(h *userHarness)
		code   domain.ErrorCode
		sentry error
	}{
		{"a user who never had an invite", func(*userHarness) {},
			domain.CodeInviteNotPending, domain.ErrInvalidInput},
		{"a user who already chose a password", func(h *userHarness) {
			used := outstandingInvite(otherUserID, fixedNow.Add(time.Hour))
			at := fixedNow
			used.ConsumedAt = &at
			h.links.latest[otherUserID] = used
		}, domain.CodeInviteNotPending, domain.ErrInvalidInput},
		{"a deactivated user", func(h *userHarness) {
			h.users.stored[otherUserID].IsActive = false
			h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))
		}, domain.CodeInviteNotPending, domain.ErrInvalidInput},
		{"an invite redeemed while the resend waited on it", func(h *userHarness) {
			h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))
			h.links.redeemedMeanwhile = true
		}, domain.CodeInviteNotPending, domain.ErrInvalidInput},
	}
	for _, tc := range refusals {
		t.Run(tc.name+" is refused", func(t *testing.T) {
			h := newUserHarness(storedAdmin(), storedSeller())
			tc.setup(h)

			err := h.svc.ResendInvite(context.Background(), adminTenant(), otherUserID)
			if !errors.Is(err, tc.sentry) || domain.CodeOf(err) != tc.code {
				t.Fatalf("ResendInvite() = %v (%s), want %s", err, domain.CodeOf(err), tc.code)
			}
			if len(h.links.created) != 0 || len(h.mail.sent) != 0 {
				t.Error("a link was minted or mailed")
			}
		})
	}

	// A double click or a second admin: the other resend has already mailed a live link.
	t.Run("a resend that loses to another resend mails nothing and succeeds", func(t *testing.T) {
		h := newUserHarness(storedAdmin(), storedSeller())
		h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))
		h.links.resentMeanwhile = true

		if err := h.svc.ResendInvite(context.Background(), adminTenant(), otherUserID); err != nil {
			t.Fatalf("ResendInvite() = %v, want no error", err)
		}
		if len(h.links.created) != 0 || len(h.mail.sent) != 0 {
			t.Errorf("minted %d and mailed %d, want neither", len(h.links.created), len(h.mail.sent))
		}
	})

	t.Run("console mail is refused before anything is read", func(t *testing.T) {
		h := newUserHarnessDelivering(false, storedAdmin(), storedSeller())
		h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))

		err := h.svc.ResendInvite(context.Background(), adminTenant(), otherUserID)
		if domain.CodeOf(err) != domain.CodeMailNotConfigured {
			t.Fatalf("ResendInvite() on console mail = %v, want %s", err, domain.CodeMailNotConfigured)
		}
		if len(h.links.created) != 0 {
			t.Error("a link was minted")
		}
	})
}

func sellerUpdate(email string) domain.UserUpdate {
	return domain.UserUpdate{
		Name: "Vendedor", Email: email, Role: domain.UserRoleSeller,
		BranchIDs: []uuid.UUID{assignedBranch},
	}
}

// A link mailed to the old address would let whoever reads that mailbox back into the account.
func TestUserService_UpdateEmailRetiresEveryLinkAndWritesToBothMailboxes(t *testing.T) {
	proved := storedSeller()
	verifiedAt := fixedNow.Add(-24 * time.Hour)
	proved.EmailVerifiedAt = &verifiedAt
	h := newUserHarness(storedAdmin(), proved)

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("nueva@corralon.test")); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.links.invalidatedAll) != 1 || h.links.invalidatedAll[0] != otherUserID {
		t.Errorf("links retired for %v, want every link of the edited user", h.links.invalidatedAll)
	}
	if !reflect.DeepEqual(h.verifier.noticesTo, []string{"v@corralon.test"}) {
		t.Errorf("old-address notices = %v, want the previous address", h.verifier.noticesTo)
	}
	if !reflect.DeepEqual(h.verifier.verificationsTo, []string{"nueva@corralon.test"}) {
		t.Errorf("verification links = %v, want the new address", h.verifier.verificationsTo)
	}
	if len(h.links.created) != 0 {
		t.Error("an invite was minted for a user who never had one")
	}
}

// The invite is the only way in an invited user has, so it follows the address rather than
// being retired with the rest.
func TestUserService_UpdateEmailOfAnInvitedUserSendsTheInviteToTheNewAddress(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())
	h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(-time.Hour))

	updated, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("nueva@corralon.test"))
	if err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if updated.InviteStatus != domain.InviteStatusPending {
		t.Errorf("invite status = %q, want %q", updated.InviteStatus, domain.InviteStatusPending)
	}
	if len(h.links.created) != 1 || h.links.created[0].Type != domain.AuthTokenTypeInvite {
		t.Fatalf("minted %v, want a fresh invite", h.links.created)
	}
	if len(h.mail.sent) != 1 || h.mail.sent[0].To != "nueva@corralon.test" {
		t.Fatalf("mailed %v, want the invite at the new address", h.mail.sent)
	}
	if len(h.verifier.verificationsTo) != 0 {
		t.Error("a separate verification link was sent beside the invite")
	}
	if len(h.verifier.noticesTo) != 0 {
		t.Error("an address never proved was told the new one")
	}
}

// Compared folded, like the unique index: a row stored before addresses were normalised still
// reads as the same mailbox.
func TestUserService_UpdateThatKeepsTheAddressMailsNothing(t *testing.T) {
	legacy := storedSeller()
	legacy.Email = "V@Corralon.test"
	h := newUserHarness(storedAdmin(), legacy)

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("v@corralon.test")); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.links.invalidatedAll) != 0 || len(h.verifier.noticesTo) != 0 ||
		len(h.verifier.verificationsTo) != 0 {
		t.Error("an edit that kept the address (in another case) retired links or sent mail")
	}
}

// A deactivated user can redeem neither an invite nor a confirmation, so the change mails nothing;
// the invite still follows the address, ready to resend once they are back.
func TestUserService_UpdateEmailOfADeactivatedUserMailsNothing(t *testing.T) {
	inactive := storedSeller()
	inactive.IsActive = false
	h := newUserHarness(storedAdmin(), inactive)
	h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))

	updated, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("nueva@corralon.test"))
	if err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.links.invalidatedAll) != 1 {
		t.Error("the deactivated user's links were not retired")
	}
	if len(h.mail.sent) != 0 || len(h.verifier.verificationsTo) != 0 || len(h.verifier.noticesTo) != 0 {
		t.Error("a deactivated user was mailed")
	}
	if len(h.links.created) != 1 || updated.InviteStatus != domain.InviteStatusPending {
		t.Errorf("minted %d invites, status %q; want the invite kept on record as pending",
			len(h.links.created), updated.InviteStatus)
	}
}

// The notice warns a mailbox the user proved; an address never proved — a typo an admin is
// correcting — would only be told the real one.
func TestUserService_UpdateEmailWarnsOnlyAnAddressThatWasProved(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("nueva@corralon.test")); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.verifier.noticesTo) != 0 {
		t.Errorf("notices = %v, want none for an unproved address", h.verifier.noticesTo)
	}
	if len(h.verifier.verificationsTo) != 1 {
		t.Error("the new address got no confirmation link")
	}
}

func TestUserService_UpdateEmailOfAPendingInviteSendsItToTheNewAddress(t *testing.T) {
	h := newUserHarness(storedAdmin(), storedSeller())
	h.links.latest[otherUserID] = outstandingInvite(otherUserID, fixedNow.Add(time.Hour))

	if _, err := h.svc.UpdateUser(context.Background(), adminTenant(), otherUserID,
		sellerUpdate("nueva@corralon.test")); err != nil {
		t.Fatalf("UpdateUser() = %v, want no error", err)
	}
	if len(h.mail.sent) != 1 || h.mail.sent[0].Event != domain.NotificationEventInvite ||
		h.mail.sent[0].To != "nueva@corralon.test" {
		t.Fatalf("mailed %v, want the invite at the new address", h.mail.sent)
	}
}

// The link is stored before the mail goes out, so a delivery that fails leaves an invite the admin
// can resend, not a failed creation.
func TestUserService_CreateWithInviteSurvivesAMailThatFails(t *testing.T) {
	h := newUserHarness(storedAdmin())
	h.mail.sendErr = errors.New("smtp: connection refused")

	created, err := h.svc.CreateUser(context.Background(), adminTenant(), invitedNewUser())
	if err != nil {
		t.Fatalf("CreateUser(invite) with failing mail = %v, want no error", err)
	}
	if created.InviteStatus != domain.InviteStatusPending || len(h.links.created) != 1 {
		t.Errorf("status %q with %d links, want a pending invite on record", created.InviteStatus,
			len(h.links.created))
	}
}
