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
	update := in
	f.updated = &update
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	return &domain.Branch{ID: branchID, AccountID: accountID, Name: in.Name,
		DefaultExpiryDays: in.DefaultExpiryDays, IsActive: active}, nil
}

func (f *fakeBranchReader) Deactivate(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) error {
	return nil
}

type fakeBranchChannels struct{ created int }

func (f *fakeBranchChannels) CreateManualEntry(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) error {
	f.created++
	return nil
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
	return NewBranchService(&fakeDB{}, branches, &fakeBranchChannels{}, 7), branches
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
