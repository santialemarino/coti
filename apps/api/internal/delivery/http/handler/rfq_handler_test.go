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

func TestToTextRFQDraftResponse_LeavesThePricingQuestionOpenAndCarriesTheCandidates(t *testing.T) {
	flagged := domain.QuoteItem{
		ID: uuid.New(), RequestedDescription: "membrana rara",
		Quantity: decimal.RequireFromString("2"), MatchStatus: domain.ItemMatchStatusNoMatch,
	}
	candidate := uuid.New()
	name := "Membrana asfáltica 4mm"
	code := "MEM-4"
	response := toTextRFQDraftResponse(domain.TextRFQDraft{
		RFQ:     domain.RFQ{ID: uuid.New(), Status: domain.RFQStatusGenerated},
		Quote:   &domain.Quote{ID: uuid.New(), CurrentStatus: domain.QuoteStatusDraft},
		Version: &domain.QuoteVersion{ID: uuid.New(), VersionNumber: 1, Total: decimal.Zero},
		Items:   []domain.QuoteItem{flagged},
		Alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{
			flagged.ID: {{
				ID: uuid.New(), QuoteItemID: flagged.ID, ProductID: &candidate,
				Type:   domain.QuoteItemAlternativeTypeProduct,
				Origin: domain.QuoteItemAlternativeOriginAI, Rank: 1,
				ConfidenceScore: decimal.NewNullDecimal(decimal.RequireFromString("0.5500")),
				Code:            &code, CanonicalName: &name,
			}},
		},
	})

	if len(response.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(response.Items))
	}
	item := response.Items[0]
	// Nothing on a draft has been valued, so whether the branch can price this line is unanswered.
	// A false here would tell the seller the line is fine when nobody has looked.
	if item.PricingUnavailable != nil {
		t.Errorf("pricing_unavailable = %v, want null on a draft", *item.PricingUnavailable)
	}
	if len(item.Alternatives) != 1 {
		t.Fatalf("alternatives = %d, want the candidate the line was decided against",
			len(item.Alternatives))
	}
	offer := item.Alternatives[0]
	if offer.CanonicalName == nil || *offer.CanonicalName != name {
		t.Errorf("offer name = %v, want %q", offer.CanonicalName, name)
	}
	if offer.Code == nil || *offer.Code != code {
		t.Errorf("offer code = %v, want %q", offer.Code, code)
	}
	if offer.Rank != 1 {
		t.Errorf("offer rank = %d, want 1", offer.Rank)
	}
	// Confidence is a decimal string at the column's scale, never a JSON number.
	if offer.ConfidenceScore == nil || *offer.ConfidenceScore != "0.5500" {
		t.Errorf("offer confidence = %v, want %q", offer.ConfidenceScore, "0.5500")
	}
	if offer.PriceSnapshot != nil {
		t.Errorf("offer price = %v, want null", *offer.PriceSnapshot)
	}
	if offer.Origin != string(domain.QuoteItemAlternativeOriginAI) {
		t.Errorf("offer origin = %q, want AI", offer.Origin)
	}
}

func TestToQuoteItemResponse_AnswersThePricingQuestionOnceValued(t *testing.T) {
	product := uuid.New()
	unpriceable := domain.QuoteItem{
		ID: uuid.New(), ProductID: &product, RequestedDescription: "2 rollos de membrana",
		Quantity: decimal.RequireFromString("2"), MatchStatus: domain.ItemMatchStatusMatched,
	}
	gap := true
	response := toQuoteItemResponse(unpriceable, nil, &gap)

	if response.PricingUnavailable == nil || !*response.PricingUnavailable {
		t.Errorf("pricing_unavailable = %v, want true: the branch has no price for this product",
			response.PricingUnavailable)
	}
	// A list field is an empty array rather than a null, so a client never has to handle both.
	if response.Alternatives == nil || len(response.Alternatives) != 0 {
		t.Errorf("alternatives = %#v, want an empty array", response.Alternatives)
	}
}

func TestToRfqDetailResponse_MapsAllFieldsFromDomainDetail(t *testing.T) {
	rfqID := uuid.New()
	itemID := uuid.New()
	productID := uuid.New()
	altProductID := uuid.New()
	versionID := uuid.New()
	quoteID := uuid.New()
	clientLabel := "Obra Norte"
	totalStr := "5000.00"

	detail := domain.RfqDetail{
		Rfq: domain.RfqListItem{
			ID:          rfqID,
			ClientLabel: &clientLabel,
			Channel:     "whatsapp",
			SellerName:  "Juan Pérez",
			BranchName:  "Matriz",
			ItemCount:   2,
			Status:      string(domain.QuoteStatusDraft),
			Total:       &totalStr,
		},
		Quote: &domain.Quote{
			ID: quoteID, RFQID: rfqID, BranchID: uuid.New(),
			CurrentStatus: domain.QuoteStatusDraft,
		},
		Version: &domain.QuoteVersion{
			ID: versionID, QuoteID: quoteID, VersionNumber: 1,
			Total: decimal.RequireFromString("5000.00"),
		},
		Items: []domain.QuoteItem{
			{
				ID: itemID, ProductID: &productID, VersionID: versionID,
				RequestedDescription: "10 bolsas de cemento",
				Quantity:             decimal.RequireFromString("10"),
				MatchStatus:          domain.ItemMatchStatusMatched,
			},
		},
		Alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{
			itemID: {{
				ID: uuid.New(), QuoteItemID: itemID, ProductID: &altProductID,
				Type:   domain.QuoteItemAlternativeTypeProduct,
				Origin: domain.QuoteItemAlternativeOriginAI, Rank: 1,
				ConfidenceScore: decimal.NewNullDecimal(decimal.RequireFromString("0.8500")),
			}},
		},
	}

	resp := toRfqDetailResponse(detail)

	if resp.Rfq.ID != rfqID {
		t.Errorf("rfq ID = %v, want %v", resp.Rfq.ID, rfqID)
	}
	if resp.Rfq.Client == nil || *resp.Rfq.Client != clientLabel {
		t.Errorf("client = %v, want %q", resp.Rfq.Client, clientLabel)
	}
	if resp.Quote == nil || resp.Quote.ID != quoteID {
		t.Errorf("quote = %v, want %v", resp.Quote, quoteID)
	}
	if resp.Version == nil || resp.Version.Total != "5000.00" {
		t.Errorf("version total = %v, want %q", resp.Version, "5000.00")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.ID != itemID {
		t.Errorf("item ID = %v, want %v", item.ID, itemID)
	}
	if item.Quantity != "10.00" {
		t.Errorf("quantity = %q, want %q", item.Quantity, "10.00")
	}
	if len(item.Alternatives) != 1 {
		t.Fatalf("alternatives = %d, want 1", len(item.Alternatives))
	}
	if resp.Alternatives == nil {
		t.Fatal("top-level alternatives map is nil, want present")
	}
	alts := resp.Alternatives[itemID.String()]
	if len(alts) != 1 {
		t.Fatalf("alternatives[%s] = %d, want 1", itemID, len(alts))
	}
	if alts[0].Rank != 1 {
		t.Errorf("alternative rank = %d, want 1", alts[0].Rank)
	}
}

func TestToRfqDetailResponse_OmitsQuoteAndVersionWhenAbsent(t *testing.T) {
	detail := domain.RfqDetail{
		Rfq: domain.RfqListItem{ID: uuid.New(), Status: string(domain.RFQStatusReceived)},
	}

	resp := toRfqDetailResponse(detail)

	if resp.Quote != nil {
		t.Errorf("quote = %v, want nil", resp.Quote)
	}
	if resp.Version != nil {
		t.Errorf("version = %v, want nil", resp.Version)
	}
	if resp.Items == nil || len(resp.Items) != 0 {
		t.Errorf("items = %v, want empty array", resp.Items)
	}
	if resp.Alternatives == nil || len(resp.Alternatives) != 0 {
		t.Errorf("alternatives = %v, want empty map", resp.Alternatives)
	}
}
