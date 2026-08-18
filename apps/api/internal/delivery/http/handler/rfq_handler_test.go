package handler

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestToTextRFQDraftResponse_MapsAnOrderThatProducedNoMaterials(t *testing.T) {
	response := toTextRFQDraftResponse(domain.TextRFQDraft{
		RFQ: domain.RFQ{ID: uuid.New(), Status: domain.RFQStatusReceived},
	})

	if response.Quote != nil || response.Version != nil {
		t.Errorf("quote/version = %#v/%#v, want nil: the order never reached GENERATED",
			response.Quote, response.Version)
	}
	if response.Items == nil || len(response.Items) != 0 {
		t.Errorf("items = %#v, want an empty array rather than a null", response.Items)
	}
	if response.RFQ.Status != string(domain.RFQStatusReceived) {
		t.Errorf("status = %q, want RECEIVED", response.RFQ.Status)
	}
}

func TestToTextRFQDraftResponse_MapsEveryLineAsADecimalString(t *testing.T) {
	productID := uuid.New()
	unit := "bolsa"
	rationale := "el cliente pidió 10 bolsas"
	response := toTextRFQDraftResponse(domain.TextRFQDraft{
		RFQ:     domain.RFQ{ID: uuid.New(), Status: domain.RFQStatusGenerated},
		Quote:   &domain.Quote{ID: uuid.New(), CurrentStatus: domain.QuoteStatusDraft},
		Version: &domain.QuoteVersion{ID: uuid.New(), VersionNumber: 1, Total: decimal.Zero},
		Items: []domain.QuoteItem{
			{
				ID: uuid.New(), ProductID: &productID,
				RequestedDescription: "10 bolsas de cemento",
				Quantity:             decimal.RequireFromString("10"),
				Unit:                 &unit,
				ConfidenceScore:      decimal.NewNullDecimal(decimal.RequireFromString("0.9137")),
				MatchStatus:          domain.ItemMatchStatusMatched,
				QuantityRationale:    &rationale,
			},
			{
				ID:                   uuid.New(),
				RequestedDescription: "arena",
				Quantity:             decimal.Zero,
				MatchStatus:          domain.ItemMatchStatusNoMatch,
			},
		},
	})

	if response.Version == nil || response.Version.Total != "0.00" {
		t.Errorf("version total = %#v, want the NUMERIC(14,2) string %q", response.Version, "0.00")
	}
	if len(response.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(response.Items))
	}

	matched := response.Items[0]
	// A JSON number would lose NUMERIC precision on the round trip, so quantities travel as strings.
	if matched.Quantity != "10.00" {
		t.Errorf("quantity = %q, want %q", matched.Quantity, "10.00")
	}
	// Four decimals, because that is the column: rendering it at money scale would round the
	// score the match was decided on.
	if matched.ConfidenceScore == nil || *matched.ConfidenceScore != "0.9137" {
		t.Errorf("confidence = %v, want %q", matched.ConfidenceScore, "0.9137")
	}
	if matched.MatchStatus != string(domain.ItemMatchStatusMatched) {
		t.Errorf("match status = %q, want MATCHED", matched.MatchStatus)
	}
	if matched.ProductID == nil || *matched.ProductID != productID {
		t.Errorf("product = %v, want the matched one", matched.ProductID)
	}
	if matched.QuantityRationale == nil || *matched.QuantityRationale != rationale {
		t.Errorf("rationale = %v, want %q", matched.QuantityRationale, rationale)
	}

	flagged := response.Items[1]
	if flagged.ProductID != nil {
		t.Errorf("product = %v on a NO_MATCH line, want null", flagged.ProductID)
	}
	// Null rather than "0.0000": nothing scored this line, and a zero would read as a candidate
	// that scored nothing.
	if flagged.ConfidenceScore != nil {
		t.Errorf("confidence = %v on an unscored line, want null", flagged.ConfidenceScore)
	}
	if flagged.MatchStatus != string(domain.ItemMatchStatusNoMatch) {
		t.Errorf("match status = %q, want NO_MATCH", flagged.MatchStatus)
	}
}
