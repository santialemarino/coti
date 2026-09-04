package domain

import (
	"testing"

	"github.com/google/uuid"
)

// BranchFilter is the single place branch reach is decided, and the nil-versus-empty
// distinction is load-bearing: the per-branch reads bind it as
// ($n::uuid[] IS NULL OR branch_id = ANY($n)), so nil reads every branch of the account and an
// empty slice reads none. Getting those two backwards is a cross-branch leak, not a bug in a
// list order — which is why it is asserted here and not only through a service.

var (
	branchA = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	branchB = uuid.MustParse("22222222-2222-4222-8222-222222222222")
)

func TestTenant_BranchFilter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		tenant  Tenant
		want    []uuid.UUID
		wantNil bool
	}{
		{
			name:   "a selected branch narrows to exactly that branch",
			tenant: Tenant{Role: UserRoleSeller, BranchID: branchA},
			want:   []uuid.UUID{branchA},
		},
		{
			name:   "a selected branch wins for an admin too",
			tenant: Tenant{Role: UserRoleAdmin, BranchID: branchA},
			want:   []uuid.UUID{branchA},
		},
		{
			// Nil, not empty: an admin with no branch selected reads the whole account.
			name:    "an admin with no branch reaches every branch",
			tenant:  Tenant{Role: UserRoleAdmin},
			wantNil: true,
		},
		{
			name:   "a seller with no branch narrows to their assignments",
			tenant: Tenant{Role: UserRoleSeller, AllowedBranchIDs: []uuid.UUID{branchA, branchB}},
			want:   []uuid.UUID{branchA, branchB},
		},
		{
			// The fail-closed case: assignments never loaded must read nothing, not everything.
			name:   "a seller whose assignments were never loaded reads nothing",
			tenant: Tenant{Role: UserRoleSeller},
			want:   []uuid.UUID{},
		},
		{
			name:   "a seller with an empty assignment set reads nothing",
			tenant: Tenant{Role: UserRoleSeller, AllowedBranchIDs: []uuid.UUID{}},
			want:   []uuid.UUID{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.tenant.BranchFilter()

			if tc.wantNil {
				if got != nil {
					t.Fatalf("BranchFilter() = %v, want nil so the read spans the account", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("BranchFilter() = nil, which reads every branch; want %v", tc.want)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("BranchFilter() = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("BranchFilter() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestTenant_IsAdminAndHasBranch(t *testing.T) {
	t.Parallel()

	admin := Tenant{Role: UserRoleAdmin}
	seller := Tenant{Role: UserRoleSeller}
	scoped := Tenant{Role: UserRoleSeller, BranchID: branchA}

	if !admin.IsAdmin() {
		t.Error("IsAdmin() = false for an ADMIN tenant")
	}
	if seller.IsAdmin() {
		t.Error("IsAdmin() = true for a SELLER tenant")
	}
	if admin.HasBranch() {
		t.Error("HasBranch() = true with no branch selected")
	}
	if !scoped.HasBranch() {
		t.Error("HasBranch() = false with a branch selected")
	}
}
