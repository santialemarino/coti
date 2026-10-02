package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var closedBranch = uuid.MustParse("55555555-5555-4555-8555-555555555555")

/*
 * fakeBranchReader answers the branch reads a BranchService performs. `all` is what the
 * account-wide read returns and `reach` what the per-user one returns, kept apart on purpose:
 * conflating them is the mistake the two reads exist to prevent.
 */
type fakeBranchReader struct {
	all         []domain.Branch
	reach       []domain.Branch
	allCalls    int
	reachCalls  int
	updated     *domain.BranchUpdate
	activeOther int
	// missing is a branch the account does not hold, so writing it matches no row.
	missing uuid.UUID
	// mailbox is what the branch's email channel holds, which the update reads back.
	mailbox func() *string
}

func (f *fakeBranchReader) ListForUser(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, _ bool,
) ([]domain.Branch, error) {
	f.reachCalls++
	return f.reach, nil
}

func (f *fakeBranchReader) ListAllForAccount(
	_ context.Context, _ repository.Querier, _ uuid.UUID,
) ([]domain.Branch, error) {
	f.allCalls++
	return f.all, nil
}

func (f *fakeBranchReader) GetByID(
	_ context.Context, _ repository.Querier, _, branchID uuid.UUID,
) (*domain.Branch, error) {
	for _, b := range f.all {
		if b.ID == branchID {
			branch := b
			return &branch, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeBranchReader) CountActiveExcluding(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (int, error) {
	return f.activeOther, nil
}

func (f *fakeBranchReader) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewBranch,
) (*domain.Branch, error) {
	return &domain.Branch{ID: uuid.New(), AccountID: accountID, Name: in.Name,
		DefaultExpiryDays: in.DefaultExpiryDays, IsActive: true}, nil
}

func (f *fakeBranchReader) Update(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID, in domain.BranchUpdate,
) (*domain.Branch, error) {
	if f.missing == branchID {
		return nil, domain.ErrNotFound
	}
	update := in
	f.updated = &update
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	branch := &domain.Branch{ID: branchID, AccountID: accountID, Name: in.Name,
		DefaultExpiryDays: in.DefaultExpiryDays, IsActive: active}
	if f.mailbox != nil {
		branch.Email = f.mailbox()
	}
	return branch, nil
}

func (f *fakeBranchReader) Deactivate(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) error {
	return nil
}

// fakeBranchChannels keeps a branch's email channels, which is all a BranchService writes after
// opening the defaults.
type fakeBranchChannels struct {
	defaults []*string
	email    []domain.Channel
}

func (f *fakeBranchChannels) ListActiveByType(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, channelType domain.ChannelType,
) ([]domain.Channel, error) {
	if channelType != domain.ChannelTypeEmail {
		return nil, nil
	}
	return f.email, nil
}

func (f *fakeBranchChannels) Create(
	_ context.Context, _ repository.Querier, accountID, branchID uuid.UUID, in domain.NewChannel,
) (*domain.Channel, error) {
	channel := domain.Channel{ID: uuid.New(), AccountID: accountID, BranchID: branchID,
		Type: in.Type, IsActive: true, Identifier: in.Identifier}
	f.email = append(f.email, channel)
	return &channel, nil
}

func (f *fakeBranchChannels) CreateDefaults(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, email *string,
) error {
	f.defaults = append(f.defaults, email)
	return nil
}

func (f *fakeBranchChannels) Update(
	_ context.Context, _ repository.Querier, _, _, channelID uuid.UUID, in domain.ChannelUpdate,
) (*domain.Channel, error) {
	for i := range f.email {
		if f.email[i].ID == channelID {
			f.email[i].Identifier = in.Identifier
			return &f.email[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

type fakeBranchCatalog struct{ seeded []uuid.UUID }

func (f *fakeBranchCatalog) AddActiveProducts(
	_ context.Context, _ repository.Querier, _, branchID uuid.UUID,
) (int64, error) {
	f.seeded = append(f.seeded, branchID)
	return 0, nil
}

func newBranchHarness() (*BranchService, *fakeBranchReader) {
	branches := &fakeBranchReader{
		all: []domain.Branch{
			{ID: assignedBranch, AccountID: testAccountID, Name: "Villa Bosch", IsActive: true},
			{ID: closedBranch, AccountID: testAccountID, Name: "Morón", IsActive: false},
		},
		reach: []domain.Branch{
			{ID: assignedBranch, AccountID: testAccountID, Name: "Villa Bosch", IsActive: true},
		},
	}
	return NewBranchService(&fakeDB{}, branches, &fakeBranchChannels{}, &fakeBranchCatalog{}, 7),
		branches
}

func sellerTenant() domain.Tenant {
	return domain.Tenant{AccountID: testAccountID, UserID: otherUserID, Role: domain.UserRoleSeller}
}

func TestBranchService_ListAllBranches_AdminSeesClosedOnes(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()

	got, err := svc.ListAllBranches(context.Background(), adminTenant())
	if err != nil {
		t.Fatalf("ListAllBranches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d branches, want 2", len(got))
	}
	if branches.allCalls != 1 || branches.reachCalls != 0 {
		t.Fatalf("read the wrong list: all=%d reach=%d", branches.allCalls, branches.reachCalls)
	}
}

/*
 * A closed branch is not one anyone may operate in, so the account-wide read is refused outright
 * rather than quietly answered with the caller's reach — a seller asking for it is asking for
 * something they do not have, and a silent substitution would hide that.
 */
func TestBranchService_ListAllBranches_SellerIsRefused(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()

	_, err := svc.ListAllBranches(context.Background(), sellerTenant())
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
	if branches.allCalls != 0 {
		t.Fatalf("the account-wide read ran anyway (%d calls)", branches.allCalls)
	}
}

// The switcher reads this one, and a closed branch must never reach it: selecting one would make
// every branch-scoped request answer 403.
func TestBranchService_ListBranches_KeepsToTheCallersReach(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()

	got, err := svc.ListBranches(context.Background(), sellerTenant())
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	for _, b := range got {
		if !b.IsActive {
			t.Fatalf("branch %q is closed and must not be offered", b.Name)
		}
	}
	if branches.reachCalls != 1 || branches.allCalls != 0 {
		t.Fatalf("read the wrong list: all=%d reach=%d", branches.allCalls, branches.reachCalls)
	}
}

// Reopening is an update that sets the flag back, so it must not be caught by the guard that
// keeps an account from closing its last active branch.
func TestBranchService_UpdateBranch_ReopeningSkipsTheLastActiveGuard(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()
	branches.activeOther = 0
	active := true

	got, err := svc.UpdateBranch(context.Background(), adminTenant(), closedBranch,
		domain.BranchUpdate{Name: "Morón", DefaultExpiryDays: 5, IsActive: &active})
	if err != nil {
		t.Fatalf("UpdateBranch: %v", err)
	}
	if !got.IsActive {
		t.Fatal("the branch was not reopened")
	}
}

// Closing the only active branch would leave the account with nowhere to operate.
func TestBranchService_DeactivateBranch_RefusesTheLastActiveOne(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()
	branches.activeOther = 0

	err := svc.DeactivateBranch(context.Background(), adminTenant(), assignedBranch)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

// Closing one that is already closed changes nothing and is not an error: the guard reads the
// branch first and lets an inactive one through.
func TestBranchService_DeactivateBranch_AlreadyClosedIsAllowed(t *testing.T) {
	t.Parallel()
	svc, branches := newBranchHarness()
	branches.activeOther = 0

	if err := svc.DeactivateBranch(context.Background(), adminTenant(), closedBranch); err != nil {
		t.Fatalf("DeactivateBranch: %v", err)
	}
}

// A new branch starts with the account's catalog available, so its first order can match.
func TestBranchService_CreateBranch_MakesTheCatalogAvailableAtTheNewBranch(t *testing.T) {
	t.Parallel()
	catalog := &fakeBranchCatalog{}
	svc := NewBranchService(&fakeDB{}, &fakeBranchReader{}, &fakeBranchChannels{}, catalog, 7)

	branch, err := svc.CreateBranch(context.Background(), adminTenant(),
		domain.NewBranch{Name: "Sucursal Norte"})
	if err != nil {
		t.Fatalf("CreateBranch() = %v, want no error", err)
	}
	if len(catalog.seeded) != 1 || catalog.seeded[0] != branch.ID {
		t.Fatalf("catalog seeded at %v, want only the new branch %v", catalog.seeded, branch.ID)
	}
}

func strptr(s string) *string { return &s }

func TestBranchService_CreateBranch_OpensTheDefaultChannelsWithTheMailbox(t *testing.T) {
	t.Parallel()
	channels := &fakeBranchChannels{}
	svc := NewBranchService(&fakeDB{}, &fakeBranchReader{}, channels, &fakeBranchCatalog{}, 7)

	branch, err := svc.CreateBranch(context.Background(), adminTenant(),
		domain.NewBranch{Name: "Sucursal Norte", Email: strptr("  Ventas@Norte.Test ")})
	if err != nil {
		t.Fatalf("CreateBranch() = %v, want no error", err)
	}
	if len(channels.defaults) != 1 || channels.defaults[0] == nil ||
		*channels.defaults[0] != "ventas@norte.test" {
		t.Fatalf("default channels opened with %v, want the normalized mailbox", channels.defaults)
	}
	if branch.Email == nil || *branch.Email != "ventas@norte.test" {
		t.Fatalf("branch email = %v, want the normalized mailbox", branch.Email)
	}
}

func TestBranchService_CreateBranch_RefusesAMailboxThatIsNotAnAddress(t *testing.T) {
	t.Parallel()
	svc := NewBranchService(&fakeDB{}, &fakeBranchReader{}, &fakeBranchChannels{},
		&fakeBranchCatalog{}, 7)

	_, err := svc.CreateBranch(context.Background(), adminTenant(),
		domain.NewBranch{Name: "Sucursal Norte", Email: strptr("ventas en norte")})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateBranch() = %v, want ErrInvalidInput", err)
	}
}

func TestBranchService_UpdateBranch_WritesTheMailboxOntoTheEmailChannel(t *testing.T) {
	t.Parallel()
	channels := &fakeBranchChannels{email: []domain.Channel{
		{ID: uuid.New(), Type: domain.ChannelTypeEmail, IsActive: true},
	}}
	svc := NewBranchService(&fakeDB{}, channels.reader(), channels, &fakeBranchCatalog{}, 7)
	update := domain.BranchUpdate{Name: "Villa Bosch", DefaultExpiryDays: 7}

	update.Email = strptr("ventas@villabosch.test")
	if _, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch, update); err != nil {
		t.Fatalf("UpdateBranch(set) = %v", err)
	}
	if got := channels.email[0].Identifier; got == nil || *got != "ventas@villabosch.test" {
		t.Fatalf("mailbox = %v, want it written", got)
	}

	update.Email = nil
	if _, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch, update); err != nil {
		t.Fatalf("UpdateBranch(untouched) = %v", err)
	}
	if channels.email[0].Identifier == nil {
		t.Fatal("an update without the field erased the mailbox")
	}

	update.Email = strptr("")
	if _, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch, update); err != nil {
		t.Fatalf("UpdateBranch(clear) = %v", err)
	}
	if channels.email[0].Identifier != nil {
		t.Fatalf("mailbox = %v, want it cleared", *channels.email[0].Identifier)
	}
}

func TestBranchService_UpdateBranch_OpensTheEmailChannelABranchLacks(t *testing.T) {
	t.Parallel()
	channels := &fakeBranchChannels{}
	svc := NewBranchService(&fakeDB{}, newBranchReaderWith(), channels, &fakeBranchCatalog{}, 7)

	_, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch,
		domain.BranchUpdate{Name: "Villa Bosch", DefaultExpiryDays: 7,
			Email: strptr("ventas@villabosch.test")})
	if err != nil {
		t.Fatalf("UpdateBranch() = %v", err)
	}
	if len(channels.email) != 1 || channels.email[0].Identifier == nil {
		t.Fatalf("email channels = %+v, want one opened with the mailbox", channels.email)
	}
}

func TestBranchService_UpdateBranch_KeepsTheMailboxAConfiguredChannelSendsFrom(t *testing.T) {
	t.Parallel()
	channels := &fakeBranchChannels{email: []domain.Channel{{ID: uuid.New(),
		Type: domain.ChannelTypeEmail, IsActive: true, IsConfigured: true,
		Identifier: strptr("ventas@villabosch.test")}}}
	svc := NewBranchService(&fakeDB{}, channels.reader(), channels, &fakeBranchCatalog{}, 7)

	_, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch,
		domain.BranchUpdate{Name: "Villa Bosch", DefaultExpiryDays: 7, Email: strptr("")})
	if !errors.Is(err, domain.ErrInvalidInput) || domain.CodeOf(err) != domain.CodeBranchMailboxRequired {
		t.Fatalf("UpdateBranch() = %v (%s), want %s", err, domain.CodeOf(err),
			domain.CodeBranchMailboxRequired)
	}
	if channels.email[0].Identifier == nil {
		t.Fatal("the configured channel lost its mailbox")
	}
}

// reader is a branch reader whose branch reads its mailbox back from these channels.
func (f *fakeBranchChannels) reader() *fakeBranchReader {
	r := newBranchReaderWith()
	r.mailbox = func() *string {
		for _, c := range f.email {
			if c.IsActive {
				return c.Identifier
			}
		}
		return nil
	}
	return r
}

func newBranchReaderWith() *fakeBranchReader {
	return &fakeBranchReader{all: []domain.Branch{
		{ID: assignedBranch, AccountID: testAccountID, Name: "Villa Bosch", IsActive: true},
	}}
}

// The update is what proves the branch is the account's: a mailbox synced before it would open a
// channel on a branch that is not there, or that belongs to another account.
func TestBranchService_UpdateBranch_WritesNoChannelForABranchTheAccountLacks(t *testing.T) {
	t.Parallel()
	foreign := uuid.New()
	branches := newBranchReaderWith()
	branches.missing = foreign
	channels := &fakeBranchChannels{}
	svc := NewBranchService(&fakeDB{}, branches, channels, &fakeBranchCatalog{}, 7)

	_, err := svc.UpdateBranch(context.Background(), adminTenant(), foreign,
		domain.BranchUpdate{Name: "Ajena", DefaultExpiryDays: 7, Email: strptr("ajena@corralon.test")})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateBranch(foreign) = %v, want %v", err, domain.ErrNotFound)
	}
	if len(channels.email) != 0 {
		t.Fatalf("email channels = %+v, want none written", channels.email)
	}
}

// The form sends the mailbox on every save, so a rename must not touch a channel the mailbox did not
// change — or a branch whose email channel cannot be written could never be renamed.
func TestBranchService_UpdateBranch_LeavesTheChannelAloneWhenTheMailboxIsUnchanged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		channels []domain.Channel
		email    string
	}{
		{"the same address, differently cased", []domain.Channel{{ID: uuid.New(),
			Type: domain.ChannelTypeEmail, IsActive: true, Identifier: strptr("ventas@villabosch.test")},
			{ID: uuid.New(), Type: domain.ChannelTypeEmail, IsActive: true}}, " Ventas@VillaBosch.test "},
		{"blank with no active email channel", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channels := &fakeBranchChannels{email: tc.channels}
			svc := NewBranchService(&fakeDB{}, channels.reader(), channels, &fakeBranchCatalog{}, 7)

			if _, err := svc.UpdateBranch(context.Background(), adminTenant(), assignedBranch,
				domain.BranchUpdate{Name: "Villa Bosch Norte", DefaultExpiryDays: 7,
					Email: strptr(tc.email)}); err != nil {
				t.Fatalf("UpdateBranch() = %v, want the rename to go through", err)
			}
			if len(channels.email) != len(tc.channels) {
				t.Fatalf("email channels = %+v, want them untouched", channels.email)
			}
		})
	}
}
