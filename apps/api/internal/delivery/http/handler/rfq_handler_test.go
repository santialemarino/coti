package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type stubRFQService struct {
	draft     *domain.TextRFQDraft
	tenant    domain.Tenant
	input     domain.TextRFQDraftInput
	fileInput domain.FileRFQDraftInput
	calls     int
}

func (s *stubRFQService) CreateTextDraft(
	_ context.Context, tenant domain.Tenant, input domain.TextRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	s.calls++
	s.tenant = tenant
	s.input = input
	return s.draft, nil
}

func (s *stubRFQService) CreateFileDraft(
	_ context.Context, tenant domain.Tenant, input domain.FileRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	s.calls++
	s.tenant = tenant
	s.fileInput = input
	return s.draft, nil
}

func (s *stubRFQService) CreateWhatsAppMockDraft(
	_ context.Context, _ domain.Tenant, _ domain.WhatsAppMockRFQInput,
) (*domain.TextRFQDraft, error) {
	return s.draft, nil
}

func TestRFQHandler_CreateTextDraft_ReturnsAReviewableDraft(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	branchID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	userID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	channelID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	productID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	rfqID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	quoteID := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	versionID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	itemID := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	unit := "bolsa"
	rationale := "el cliente pidió 10 bolsas"
	rawText := "10 bolsas de cemento"
	now := time.Date(2026, time.August, 21, 15, 0, 0, 0, time.UTC)

	service := &stubRFQService{draft: &domain.TextRFQDraft{
		RFQ: domain.RFQ{
			ID: rfqID, AccountID: accountID,
			BranchID: branchID, ChannelID: channelID, RawText: &rawText,
			Status: domain.RFQStatusGenerated, ReceivedAt: now, CreatedAt: now, UpdatedAt: now,
		},
		Quote: &domain.Quote{
			ID: quoteID, AccountID: accountID, BranchID: branchID, RFQID: rfqID, SellerID: &userID,
			CurrentVersionID: &versionID, CurrentStatus: domain.QuoteStatusDraft,
			CreatedAt: now, UpdatedAt: now,
		},
		Version: &domain.QuoteVersion{
			ID: versionID, AccountID: accountID, QuoteID: quoteID, AuthorID: &userID,
			VersionNumber: 1, Total: decimal.Zero, CreatedAt: now,
		},
		Items: []domain.QuoteItem{{
			ID: itemID, AccountID: accountID, VersionID: versionID, ProductID: &productID,
			RequestedDescription: rawText, Quantity: decimal.RequireFromString("10"), Unit: &unit,
			ConfidenceScore: decimal.NewNullDecimal(decimal.RequireFromString("0.9100")),
			MatchStatus:     domain.ItemMatchStatusMatched, QuantityRationale: &rationale, CreatedAt: now,
		}},
	}}
	handler := NewRFQHandler(service, 10<<20)
	payload, err := json.Marshal(map[string]any{
		"channel_id": channelID,
		"raw_text":   rawText,
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rfqs/text-drafts", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	tenant := domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: userID, Role: domain.UserRoleAdmin,
	}
	middleware.SetTenant(c, tenant)

	handler.CreateTextDraft(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if service.calls != 1 || service.tenant.AccountID != tenant.AccountID ||
		service.tenant.BranchID != tenant.BranchID || service.tenant.UserID != tenant.UserID ||
		service.tenant.Role != tenant.Role {
		t.Fatalf("service calls/tenant = %d/%+v, want one call for %+v", service.calls,
			service.tenant, tenant)
	}
	if service.input.ChannelID != channelID || service.input.RawText != rawText {
		t.Errorf("service input = %+v, want channel %s and original text", service.input, channelID)
	}

	var response dto.TextRFQDraftResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %s: %v", recorder.Body, err)
	}
	if response.RFQ.Status != string(domain.RFQStatusGenerated) {
		t.Errorf("rfq status = %q, want GENERATED", response.RFQ.Status)
	}
	if response.Quote == nil || response.Quote.CurrentStatus != string(domain.QuoteStatusDraft) {
		t.Errorf("quote = %+v, want DRAFT", response.Quote)
	}
	if response.Version == nil || response.Version.Total != "0.00" {
		t.Errorf("version = %+v, want an unpriced version at 0.00", response.Version)
	}
	if len(response.Items) != 1 || response.Items[0].Quantity != "10.00" ||
		response.Items[0].MatchStatus != string(domain.ItemMatchStatusMatched) {
		t.Errorf("items = %+v, want one matched line with quantity 10.00", response.Items)
	}
	t.Logf("response: %s", recorder.Body.String())
}

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
	if matched.RequestedDescription != "10 bolsas de cemento" {
		t.Errorf("description = %q, want the client's words", matched.RequestedDescription)
	}
	if matched.Unit == nil || *matched.Unit != "bolsa" {
		t.Errorf("unit = %v, want %q", matched.Unit, "bolsa")
	}
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
	if flagged.Quantity != "0.00" {
		t.Errorf("quantity = %q on an unresolved line, want %q", flagged.Quantity, "0.00")
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
	quoteNumber := int64(42)
	changedAt := time.Date(2026, time.September, 4, 14, 30, 0, 0, time.UTC)
	previousRFQ := domain.RFQStatusReceived
	previousQuote := domain.QuoteStatusQuoted
	sentAt := changedAt.Add(5 * time.Minute)
	expiresAt := changedAt.AddDate(0, 0, 7)

	detail := domain.RfqDetail{
		Rfq: domain.RfqListItem{
			ID:          rfqID,
			QuoteNumber: &quoteNumber,
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
		RFQStatusChanges: []domain.RFQStatusChange{{
			ID: uuid.New(), RFQID: rfqID, PreviousStatus: &previousRFQ,
			NewStatus: domain.RFQStatusGenerated, UserID: &productID,
			ChangedAt: changedAt, CreatedAt: changedAt,
		}},
		QuoteStatusChanges: []domain.QuoteStatusChange{{
			ID: uuid.New(), QuoteID: quoteID, PreviousStatus: &previousQuote,
			NewStatus: domain.QuoteStatusSent, UserID: &productID,
			ChangedAt: sentAt, CreatedAt: sentAt,
		}},
		Deliveries: []domain.QuoteSend{{
			ID: uuid.New(), VersionID: versionID, ChannelType: domain.ChannelTypeWhatsApp,
			Destination: "+5491155550101", Format: domain.SendFormatWebAppLink,
			TrackingStatus: domain.SendTrackingStatusDelivered, SentAt: &sentAt,
			ExpiresAt: &expiresAt, CreatedAt: changedAt,
		}},
	}

	resp := toRfqDetailResponse(detail)

	if resp.Rfq.ID != rfqID {
		t.Errorf("rfq ID = %v, want %v", resp.Rfq.ID, rfqID)
	}
	if resp.Rfq.QuoteNumber == nil || *resp.Rfq.QuoteNumber != quoteNumber {
		t.Errorf("quote number = %v, want %d", resp.Rfq.QuoteNumber, quoteNumber)
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
	if len(resp.RFQHistory) != 1 ||
		resp.RFQHistory[0].PreviousStatus == nil ||
		*resp.RFQHistory[0].PreviousStatus != string(domain.RFQStatusReceived) ||
		resp.RFQHistory[0].NewStatus != string(domain.RFQStatusGenerated) {
		t.Errorf("RFQ history = %+v, want RECEIVED -> GENERATED", resp.RFQHistory)
	}
	if len(resp.QuoteHistory) != 1 ||
		resp.QuoteHistory[0].PreviousStatus == nil ||
		*resp.QuoteHistory[0].PreviousStatus != string(domain.QuoteStatusQuoted) ||
		resp.QuoteHistory[0].NewStatus != string(domain.QuoteStatusSent) {
		t.Errorf("quote history = %+v, want QUOTED -> SENT", resp.QuoteHistory)
	}
	if len(resp.Deliveries) != 1 ||
		resp.Deliveries[0].TrackingStatus != string(domain.SendTrackingStatusDelivered) ||
		resp.Deliveries[0].Channel != string(domain.ChannelTypeWhatsApp) ||
		resp.Deliveries[0].ExpiresAt == nil {
		t.Errorf("deliveries = %+v, want one delivered WhatsApp send with expiry",
			resp.Deliveries)
	}
}

func TestToRfqDetailResponse_MapsTheChangeRequestDiff(t *testing.T) {
	reason := "entrega el viernes"
	price := "8500.00"
	detail := domain.RfqDetail{
		Rfq: domain.RfqListItem{ID: uuid.New(), Status: string(domain.QuoteStatusChangeRequested)},
		ChangesRequested: &domain.ChangeRequestDiff{
			Reason: &reason,
			Original: domain.ChangeRequestSide{
				Items: []domain.DiffLineItem{{
					Description: "10 bolsas de cemento", Quantity: "10.00",
					UnitPrice: &price,
				}},
				Discounts: []domain.DiffDiscountLine{{Name: "Descuento obra", Amount: "500.00"}},
				Total:     decimal.RequireFromString("16000.00"),
			},
			Requested: domain.ChangeRequestSide{
				Items: []domain.DiffLineItem{{
					Description: "10 bolsas de cemento", Quantity: "12.00",
					UnitPrice: &price, Changed: true, ChangeType: "modified",
				}},
				Discounts: []domain.DiffDiscountLine{{Name: "Descuento obra", Amount: "800.00", Changed: true}},
				Total:     decimal.RequireFromString("17000.00"),
			},
		},
	}

	resp := toRfqDetailResponse(detail)

	if resp.ChangesRequested == nil {
		t.Fatal("changes_requested is nil, want the mapped diff")
	}
	if resp.ChangesRequested.Reason == nil || *resp.ChangesRequested.Reason != reason {
		t.Errorf("reason = %v, want %q", resp.ChangesRequested.Reason, reason)
	}
	if resp.ChangesRequested.Original.Total != "16000.00" || resp.ChangesRequested.Requested.Total != "17000.00" {
		t.Errorf("totals = original %q requested %q, want 16000.00 and 17000.00",
			resp.ChangesRequested.Original.Total, resp.ChangesRequested.Requested.Total)
	}
	requested := resp.ChangesRequested.Requested.Items[0]
	if !requested.Changed || requested.ChangeType == nil || *requested.ChangeType != "modified" {
		t.Errorf("requested item = changed %v change_type %v, want true modified",
			requested.Changed, requested.ChangeType)
	}
	original := resp.ChangesRequested.Original.Items[0]
	if original.Changed || original.ChangeType != nil {
		t.Errorf("original item = changed %v change_type %v, want neutral",
			original.Changed, original.ChangeType)
	}
	if !resp.ChangesRequested.Requested.Discounts[0].Changed {
		t.Error("requested discount not flagged")
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
	if resp.RFQHistory == nil || len(resp.RFQHistory) != 0 {
		t.Errorf("RFQ history = %v, want empty array", resp.RFQHistory)
	}
	if resp.QuoteHistory == nil || len(resp.QuoteHistory) != 0 {
		t.Errorf("quote history = %v, want empty array", resp.QuoteHistory)
	}
	if resp.Deliveries == nil || len(resp.Deliveries) != 0 {
		t.Errorf("deliveries = %v, want empty array", resp.Deliveries)
	}
}

// rfqSetSellerTenant is the admin tenant sharing the seller-set handler tests.
func rfqSetSellerTenant() domain.Tenant {
	return domain.Tenant{
		AccountID: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		BranchID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		UserID:    uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Role:      domain.UserRoleAdmin,
	}
}

// The full admin steering path: the route reads the operator's identity, the path id and the
// body id, forwards all three, and the quote comes back stamped.
func TestRfqHandler_SetSeller_RoutesTheAdminRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := rfqSetSellerTenant()
	targetID := uuid.New()
	service := &stubDiscountRFQService{
		setSeller: &domain.Quote{
			ID: uuid.New(), AccountID: tenant.AccountID, BranchID: tenant.BranchID,
			RFQID: tenant.AccountID, SellerID: &targetID,
		},
	}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "rfqId", Value: tenant.AccountID.String()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/v1/rfqs/"+tenant.AccountID.String()+"/seller",
		bytes.NewReader([]byte(`{"seller_id":"`+targetID.String()+`"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, tenant)

	handler.SetSeller(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if !equalTenant(service.setSellerTenant, tenant) {
		t.Errorf("service tenant = %+v, want %+v", service.setSellerTenant, tenant)
	}
	if service.setSellerRFQID != tenant.AccountID {
		t.Errorf("service rfq = %v, want %v", service.setSellerRFQID, tenant.AccountID)
	}
	if service.setSellerID == nil || *service.setSellerID != targetID {
		t.Errorf("service seller = %v, want %v", service.setSellerID, targetID)
	}

	var response dto.QuoteResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %s: %v", recorder.Body, err)
	}
	if response.SellerID == nil || *response.SellerID != targetID {
		t.Errorf("response seller_id = %v, want %v", response.SellerID, targetID)
	}
}

// A missing or null seller_id in the body clears the assignment rather than naming anyone.
func TestRfqHandler_SetSeller_NullBodyClears(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := rfqSetSellerTenant()
	service := &stubDiscountRFQService{
		setSeller: &domain.Quote{ID: uuid.New(), AccountID: tenant.AccountID, BranchID: tenant.BranchID},
	}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "rfqId", Value: tenant.AccountID.String()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/v1/rfqs/"+tenant.AccountID.String()+"/seller",
		bytes.NewReader([]byte(`{"seller_id":null}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, tenant)

	handler.SetSeller(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if service.setSellerID != nil {
		t.Errorf("service seller = %v, want nil (clear)", service.setSellerID)
	}
}

// A body that is not the envelope reaches bind validation, which answers 400 before the
// service sees anything.
func TestRfqHandler_SetSeller_BadBodyIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := rfqSetSellerTenant()
	service := &stubDiscountRFQService{}
	handler := NewRfqHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "rfqId", Value: tenant.AccountID.String()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/v1/rfqs/"+tenant.AccountID.String()+"/seller",
		bytes.NewReader([]byte(`{"seller_id":123}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetTenant(c, tenant)

	handler.SetSeller(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body)
	}
	if recorder.Body.String() != "" {
		var envelope dto.ErrorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Code != string(domain.CodeInvalidBody) {
			t.Errorf("error code = %q, want INVALID_BODY", envelope.Code)
		}
	}
}

// The service refusals surface unchanged: a foreign seller is 422 and the caller's role 403 —
// the latter never normally reaches this route because RequireAdmin sits before it.
func TestRfqHandler_SetSeller_ServiceRefusalsPassThrough(t *testing.T) {
	for name, want := range map[string]struct {
		err    error
		status int
	}{
		"foreign seller": {err: fmt.Errorf("%w: seller_id names a seller who cannot serve the order's branch", domain.ErrInvalidInput), status: http.StatusUnprocessableEntity},
		"not an admin":   {err: domain.ErrForbidden, status: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			tenant := rfqSetSellerTenant()
			service := &stubDiscountRFQService{setSellerErr: want.err}
			handler := NewRfqHandler(service)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Params = gin.Params{{Key: "rfqId", Value: tenant.AccountID.String()}}
			c.Request = httptest.NewRequest(http.MethodPut, "/v1/rfqs/"+tenant.AccountID.String()+"/seller",
				bytes.NewReader([]byte(`{"seller_id":"`+uuid.New().String()+`"}`)))
			c.Request.Header.Set("Content-Type", "application/json")
			middleware.SetTenant(c, tenant)

			handler.SetSeller(c)

			if recorder.Code != want.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, want.status, recorder.Body)
			}
		})
	}
}
