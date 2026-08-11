package services

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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

type userHarness struct {
	svc         *UserService
	db          *fakeDB
	users       *fakeAdminUsers
	assignments *fakeAssignments
	branches    *fakeBranchExistence
}

func newUserHarness(stored ...*domain.AppUser) *userHarness {
	db := &fakeDB{}
	users := newFakeAdminUsers(stored...)
	assignments := newFakeAssignments()
	branches := &fakeBranchExistence{known: []uuid.UUID{assignedBranch}}
	return &userHarness{
		svc:         NewUserService(db, users, assignments, branches, testAuthConfig()),
		db:          db,
		users:       users,
		assignments: assignments,
		branches:    branches,
	}
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

// An admin-created user is trusted on the admin's word, in the same transaction that created
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

// Editing your own profile stays allowed — only the role and the active flag are guarded.
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
