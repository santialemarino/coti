package services

import (
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"testing"
)

func TestFollowupTemplate_RenderAndValidation(t *testing.T) {
	tests := []struct {
		template string
		valid    bool
	}{{domain.DefaultFollowupTemplate, true}, {"{cliente}: {cotizacion}", true}, {"", false}, {"{total}", false}}
	for _, tc := range tests {
		if err := domain.ValidateFollowupTemplate(tc.template); (err == nil) != tc.valid {
			t.Errorf("ValidateFollowupTemplate(%q) = %v", tc.template, err)
		}
	}
	if got := domain.RenderFollowupTemplate("{cliente}: {cotizacion}", "Cliente", 7); got != "Cliente: COT-000007" {
		t.Fatalf("rendered = %q", got)
	}
}
