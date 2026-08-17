package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestToTextRFQDraftResponse_MapsClarificationWithoutQuote(t *testing.T) {
	rfqID := uuid.New()
	response := toTextRFQDraftResponse(domain.TextRFQDraft{
		RFQ: domain.RFQ{ID: rfqID, Status: domain.RFQStatusReceived},
		Clarifications: []domain.RFQClarification{{
			ID: uuid.New(), RFQID: rfqID,
			IssueType:            domain.RFQClarificationMissingQuantity,
			RequestedDescription: "cemento",
			Question:             "Cuantas bolsas de cemento necesitas?",
			Reason:               "La cantidad no aparece en el pedido.",
			Status:               domain.RFQClarificationStatusProposed,
		}},
	})

	if response.Quote != nil || response.Version != nil {
		t.Errorf("quote/version = %#v/%#v, want nil while clarification blocks",
			response.Quote, response.Version)
	}
	if response.Items == nil || len(response.Items) != 0 {
		t.Errorf("items = %#v, want an empty array", response.Items)
	}
	if len(response.Clarifications) != 1 ||
		response.Clarifications[0].Status != string(domain.RFQClarificationStatusProposed) {
		t.Errorf("clarifications = %#v, want one PROPOSED question", response.Clarifications)
	}
}
