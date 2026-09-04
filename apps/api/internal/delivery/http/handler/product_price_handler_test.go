package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestToProductPriceImportPreviewResponse_EmptyErrorsSerializeAsArray(t *testing.T) {
	response := toProductPriceImportPreviewResponse(&domain.ProductPriceImportPreview{
		Rows: []domain.ProductPriceImportRow{{RowNumber: 2}},
	})

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(payload), `"errors":[]`) {
		t.Fatalf("json.Marshal() = %s, want errors as an empty array", payload)
	}
}
