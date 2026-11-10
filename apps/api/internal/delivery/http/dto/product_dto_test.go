package dto

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
)

func TestListProductsQuery_ClassificationFilters(t *testing.T) {
	familyID := uuid.MustParse("50417db6-f4b2-45eb-9dd4-bc4ab45ee861")
	subgroupID := uuid.MustParse("bf81c5cb-078e-42f5-9992-60193cf95d32")
	tests := []struct {
		name       string
		query      string
		familyID   *uuid.UUID
		subgroupID *uuid.UUID
		wantError  bool
	}{
		{name: "no classification"},
		{name: "family only", query: "&family_id=" + familyID.String(), familyID: &familyID},
		{name: "subgroup only", query: "&subgroup_id=" + subgroupID.String(), subgroupID: &subgroupID},
		{
			name: "compatible alternatives", query: "&family_id=" + familyID.String() + "&subgroup_id=" + subgroupID.String(),
			familyID: &familyID, subgroupID: &subgroupID,
		},
		{name: "invalid family", query: "&family_id=invalid", wantError: true},
		{name: "invalid subgroup", query: "&family_id=" + familyID.String() + "&subgroup_id=invalid", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/products?limit=50"+tt.query, nil)
			var query ListProductsQuery
			err := binding.Query.Bind(request, &query)
			if (err != nil) != tt.wantError {
				t.Fatalf("Bind() error = %v, wantError = %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if query.Limit != 50 {
				t.Fatalf("Limit = %d, want 50", query.Limit)
			}
			for name, ids := range map[string][2]*uuid.UUID{
				"family": {query.FamilyID, tt.familyID}, "subgroup": {query.SubgroupID, tt.subgroupID},
			} {
				got, want := ids[0], ids[1]
				if (got == nil) != (want == nil) || (got != nil && want != nil && *got != *want) {
					t.Errorf("%s = %v, want %v", name, got, want)
				}
			}
		})
	}
}
