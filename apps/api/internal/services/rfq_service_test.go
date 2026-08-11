package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var (
	testRFQID     = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	testQuoteID   = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	testVersionID = uuid.MustParse("66666666-6666-4666-8666-666666666666")
	testChannelID = uuid.MustParse("77777777-7777-4777-8777-777777777777")
)

type fakeRFQExtractor struct {
	lines        []domain.ExtractedRFQLine
	err          error
	calls        int
	raw          string
	db           *fakeDB
	calledBefore bool
}

func (f *fakeRFQExtractor) Extract(_ context.Context, raw string) ([]domain.ExtractedRFQLine, error) {
	f.calls++
	f.raw = raw
	if f.db != nil {
		f.calledBefore = len(f.db.scopes) == 0
	}
	return f.lines, f.err
}

type rfqStatusChangeCall struct {
	rfqID          uuid.UUID
	previousStatus *domain.RFQStatus
	newStatus      domain.RFQStatus
	userID         *uuid.UUID
}

type fakeRFQs struct {
	created       []domain.NewRFQ
	updatedStatus []domain.RFQStatus
	statusChanges []rfqStatusChangeCall
}

func (f *fakeRFQs) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewRFQ,
) (*domain.RFQ, error) {
	f.created = append(f.created, in)
	return &domain.RFQ{
		ID: testRFQID, AccountID: accountID, BranchID: in.BranchID, ClientID: in.ClientID,
		ChannelID: in.ChannelID, RawText: in.RawText, Status: in.Status, WorkType: in.WorkType,
		ClientLabel: in.ClientLabel,
	}, nil
}

func (f *fakeRFQs) UpdateStatus(
	_ context.Context, _ repository.Querier, accountID, id uuid.UUID, status domain.RFQStatus,
) (*domain.RFQ, error) {
	f.updatedStatus = append(f.updatedStatus, status)
	return &domain.RFQ{ID: id, AccountID: accountID, BranchID: testBranchID, ChannelID: testChannelID, Status: status}, nil
}

func (f *fakeRFQs) AppendStatusChange(
	_ context.Context, _ repository.Querier, accountID, rfqID uuid.UUID,
	previousStatus *domain.RFQStatus, newStatus domain.RFQStatus, userID *uuid.UUID,
) (*domain.RFQStatusChange, error) {
	f.statusChanges = append(f.statusChanges, rfqStatusChangeCall{
		rfqID: rfqID, previousStatus: previousStatus, newStatus: newStatus, userID: userID,
	})
	return &domain.RFQStatusChange{
		AccountID: accountID, RFQID: rfqID, PreviousStatus: previousStatus,
		NewStatus: newStatus, UserID: userID,
	}, nil
}

type quoteStatusChangeCall struct {
	quoteID        uuid.UUID
	previousStatus *domain.QuoteStatus
	newStatus      domain.QuoteStatus
	userID         *uuid.UUID
}

type fakeQuoteDrafts struct {
	created        []domain.NewQuote
	currentVersion []uuid.UUID
	versions       []domain.NewQuoteVersion
	itemBatches    [][]domain.NewQuoteItem
	statusChanges  []quoteStatusChangeCall
}

func (f *fakeQuoteDrafts) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewQuote,
) (*domain.Quote, error) {
	f.created = append(f.created, in)
	return &domain.Quote{
		ID: testQuoteID, AccountID: accountID, BranchID: in.BranchID, ClientID: in.ClientID,
		RFQID: in.RFQID, SellerID: in.SellerID, CurrentStatus: in.CurrentStatus,
		ExpiresAt: in.ExpiresAt,
	}, nil
}

func (f *fakeQuoteDrafts) UpdateCurrentVersion(
	_ context.Context, _ repository.Querier, accountID, quoteID, versionID uuid.UUID,
) (*domain.Quote, error) {
	f.currentVersion = append(f.currentVersion, versionID)
	return &domain.Quote{
		ID: quoteID, AccountID: accountID, BranchID: testBranchID, RFQID: testRFQID,
		SellerID: &testUserID, CurrentVersionID: &versionID, CurrentStatus: domain.QuoteStatusDraft,
	}, nil
}

func (f *fakeQuoteDrafts) CreateVersion(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewQuoteVersion,
) (*domain.QuoteVersion, error) {
	f.versions = append(f.versions, in)
	return &domain.QuoteVersion{
		ID: testVersionID, AccountID: accountID, QuoteID: in.QuoteID, AuthorID: in.AuthorID,
		VersionNumber: in.VersionNumber, Total: in.Total, IsImmutable: in.IsImmutable,
		Comment: in.Comment,
	}, nil
}

func (f *fakeQuoteDrafts) CreateItems(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	items []domain.NewQuoteItem,
) ([]domain.QuoteItem, error) {
	f.itemBatches = append(f.itemBatches, items)
	created := make([]domain.QuoteItem, 0, len(items))
	for i, item := range items {
		created = append(created, domain.QuoteItem{
			ID: uuid.New(), AccountID: accountID, VersionID: versionID, ProductID: item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity, Unit: item.Unit,
			MatchStatus: item.MatchStatus, QuantityRationale: item.QuantityRationale,
			CreatedAt: fixedNow.AddDate(0, 0, i),
		})
	}
	return created, nil
}

func (f *fakeQuoteDrafts) AppendStatusChange(
	_ context.Context, _ repository.Querier, accountID, quoteID uuid.UUID,
	previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID,
) (*domain.QuoteStatusChange, error) {
	f.statusChanges = append(f.statusChanges, quoteStatusChangeCall{
		quoteID: quoteID, previousStatus: previousStatus, newStatus: newStatus, userID: userID,
	})
	return &domain.QuoteStatusChange{
		AccountID: accountID, QuoteID: quoteID, PreviousStatus: previousStatus,
		NewStatus: newStatus, UserID: userID,
	}, nil
}

type rfqHarness struct {
	service   *RFQService
	db        *fakeDB
	extractor *fakeRFQExtractor
	rfqs      *fakeRFQs
	quotes    *fakeQuoteDrafts
}

func newRFQHarness(lines []domain.ExtractedRFQLine) *rfqHarness {
	db := &fakeDB{}
	extractor := &fakeRFQExtractor{lines: lines, db: db}
	h := &rfqHarness{
		db: db, extractor: extractor, rfqs: &fakeRFQs{}, quotes: &fakeQuoteDrafts{},
	}
	h.service = NewRFQService(h.db, h.rfqs, h.quotes, h.extractor)
	return h
}

func TestRFQService_CreateTextDraft_PersistsGeneratedDraft(t *testing.T) {
	unit := " bag "
	rationale := " client asked for bags "
	clientLabel := " North job "
	workType := " extension "
	h := newRFQHarness([]domain.ExtractedRFQLine{
		{
			RequestedDescription: " Portland Cement ",
			Quantity:             decimal.RequireFromString("10.00"),
			Unit:                 &unit,
			QuantityRationale:    &rationale,
		},
		{
			RequestedDescription: "Sand",
			Quantity:             decimal.RequireFromString("3"),
		},
	})

	draft, err := h.service.CreateTextDraft(context.Background(), branchTenant(), domain.TextRFQDraftInput{
		ChannelID: testChannelID, ClientLabel: &clientLabel, RawText: "  Need cement and sand  ",
		WorkType: &workType,
	})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want no error", err)
	}

	if h.extractor.calls != 1 || h.extractor.raw != "Need cement and sand" {
		t.Fatalf("extractor calls/raw = %d/%q, want one call with trimmed raw text",
			h.extractor.calls, h.extractor.raw)
	}
	if !h.extractor.calledBefore {
		t.Fatal("extractor ran after the transaction opened; provider calls must stay outside it")
	}
	if len(h.db.scopes) != 1 || h.db.scopes[0] != testAccountID {
		t.Fatalf("tenant scopes = %v, want [%v]", h.db.scopes, testAccountID)
	}

	if len(h.rfqs.created) != 1 {
		t.Fatalf("rfqs created = %d, want 1", len(h.rfqs.created))
	}
	createdRFQ := h.rfqs.created[0]
	if createdRFQ.BranchID != testBranchID || createdRFQ.ChannelID != testChannelID {
		t.Errorf("rfq branch/channel = %v/%v, want %v/%v", createdRFQ.BranchID,
			createdRFQ.ChannelID, testBranchID, testChannelID)
	}
	if createdRFQ.RawText == nil || *createdRFQ.RawText != "Need cement and sand" {
		t.Errorf("rfq raw_text = %v, want trimmed original text", createdRFQ.RawText)
	}
	if createdRFQ.Status != domain.RFQStatusReceived {
		t.Errorf("rfq initial status = %s, want %s", createdRFQ.Status, domain.RFQStatusReceived)
	}
	if createdRFQ.ClientLabel == nil || *createdRFQ.ClientLabel != "North job" {
		t.Errorf("client_label = %v, want trimmed label", createdRFQ.ClientLabel)
	}
	if createdRFQ.WorkType == nil || *createdRFQ.WorkType != "extension" {
		t.Errorf("work_type = %v, want trimmed work type", createdRFQ.WorkType)
	}

	if len(h.quotes.created) != 1 {
		t.Fatalf("quotes created = %d, want 1", len(h.quotes.created))
	}
	createdQuote := h.quotes.created[0]
	if createdQuote.CurrentStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status = %s, want %s", createdQuote.CurrentStatus, domain.QuoteStatusDraft)
	}
	if createdQuote.SellerID == nil || *createdQuote.SellerID != testUserID {
		t.Errorf("seller_id = %v, want caller %v", createdQuote.SellerID, testUserID)
	}

	if len(h.quotes.versions) != 1 {
		t.Fatalf("versions created = %d, want 1", len(h.quotes.versions))
	}
	version := h.quotes.versions[0]
	if version.VersionNumber != 1 || version.IsImmutable {
		t.Errorf("version number/immutable = %d/%v, want 1/false", version.VersionNumber,
			version.IsImmutable)
	}
	if !version.Total.IsZero() {
		t.Errorf("version total = %s, want zero before pricing", version.Total)
	}

	if len(h.quotes.itemBatches) != 1 || len(h.quotes.itemBatches[0]) != 2 {
		t.Fatalf("item batches = %#v, want one batch with two items", h.quotes.itemBatches)
	}
	firstItem := h.quotes.itemBatches[0][0]
	if firstItem.ProductID != nil {
		t.Errorf("product_id = %v, want nil until catalog matching runs", firstItem.ProductID)
	}
	if firstItem.MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Errorf("match_status = %s, want %s", firstItem.MatchStatus, domain.ItemMatchStatusNoMatch)
	}
	if firstItem.RequestedDescription != "Portland Cement" {
		t.Errorf("requested_description = %q, want trimmed description", firstItem.RequestedDescription)
	}
	if firstItem.Unit == nil || *firstItem.Unit != "bag" {
		t.Errorf("unit = %v, want trimmed unit", firstItem.Unit)
	}
	if firstItem.QuantityRationale == nil || *firstItem.QuantityRationale != "client asked for bags" {
		t.Errorf("quantity_rationale = %v, want trimmed rationale", firstItem.QuantityRationale)
	}

	if len(h.rfqs.statusChanges) != 1 {
		t.Fatalf("rfq status changes = %d, want 1", len(h.rfqs.statusChanges))
	}
	rfqChange := h.rfqs.statusChanges[0]
	if rfqChange.previousStatus == nil || *rfqChange.previousStatus != domain.RFQStatusReceived ||
		rfqChange.newStatus != domain.RFQStatusGenerated {
		t.Errorf("rfq status change = %#v, want RECEIVED -> GENERATED", rfqChange)
	}
	if len(h.quotes.statusChanges) != 1 {
		t.Fatalf("quote status changes = %d, want 1", len(h.quotes.statusChanges))
	}
	quoteChange := h.quotes.statusChanges[0]
	if quoteChange.previousStatus != nil || quoteChange.newStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status change = %#v, want nil -> DRAFT", quoteChange)
	}

	if draft.RFQ.Status != domain.RFQStatusGenerated {
		t.Errorf("draft rfq status = %s, want %s", draft.RFQ.Status, domain.RFQStatusGenerated)
	}
	if draft.Quote.CurrentVersionID == nil || *draft.Quote.CurrentVersionID != testVersionID {
		t.Errorf("draft current_version_id = %v, want %v", draft.Quote.CurrentVersionID, testVersionID)
	}
	if len(draft.Items) != 2 || draft.Items[0].MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Errorf("draft items = %#v, want persisted no-match items", draft.Items)
	}
}

func TestRFQService_CreateTextDraft_RejectsWithoutBranch(t *testing.T) {
	h := newRFQHarness(validExtractedLines())
	accountWide := domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin}

	_, err := h.service.CreateTextDraft(context.Background(), accountWide, domain.TextRFQDraftInput{
		ChannelID: testChannelID, RawText: "cement",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateTextDraft() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if h.extractor.calls != 0 {
		t.Errorf("extractor calls = %d, want none for a request without active branch", h.extractor.calls)
	}
	if len(h.db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none for rejected input", h.db.scopes)
	}
}

func TestRFQService_CreateTextDraft_RejectsInvalidInputsBeforePersisting(t *testing.T) {
	longUnit := strings.Repeat("u", 65)
	cases := []struct {
		name  string
		input domain.TextRFQDraftInput
		lines []domain.ExtractedRFQLine
	}{
		{
			name:  "missing channel",
			input: domain.TextRFQDraftInput{RawText: "cement"},
			lines: validExtractedLines(),
		},
		{
			name:  "blank text",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "  "},
			lines: validExtractedLines(),
		},
		{
			name:  "no extracted lines",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cement"},
			lines: nil,
		},
		{
			name:  "blank extracted description",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cement"},
			lines: []domain.ExtractedRFQLine{{RequestedDescription: "  ", Quantity: decimal.NewFromInt(1)}},
		},
		{
			name:  "zero quantity",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cement"},
			lines: []domain.ExtractedRFQLine{{RequestedDescription: "cement", Quantity: decimal.Zero}},
		},
		{
			name:  "too many quantity decimals",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cement"},
			lines: []domain.ExtractedRFQLine{
				{RequestedDescription: "cement", Quantity: decimal.RequireFromString("1.111")},
			},
		},
		{
			name:  "unit too long",
			input: domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cement"},
			lines: []domain.ExtractedRFQLine{
				{RequestedDescription: "cement", Quantity: decimal.NewFromInt(1), Unit: &longUnit},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRFQHarness(tc.lines)

			_, err := h.service.CreateTextDraft(context.Background(), branchTenant(), tc.input)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("CreateTextDraft() = %v, want %v", err, domain.ErrInvalidInput)
			}
			if len(h.db.scopes) != 0 {
				t.Errorf("tenant scopes = %v, want none for invalid input", h.db.scopes)
			}
			if len(h.rfqs.created) != 0 || len(h.quotes.created) != 0 {
				t.Errorf("persisted rfqs/quotes = %d/%d, want none", len(h.rfqs.created),
					len(h.quotes.created))
			}
		})
	}
}

func TestRFQService_CreateTextDraft_PropagatesExtractorErrorBeforePersisting(t *testing.T) {
	h := newRFQHarness(validExtractedLines())
	h.extractor.err = domain.ErrInvalidInput

	_, err := h.service.CreateTextDraft(context.Background(), branchTenant(), domain.TextRFQDraftInput{
		ChannelID: testChannelID, RawText: "cement",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateTextDraft() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.db.scopes) != 0 {
		t.Errorf("tenant scopes = %v, want none when extraction fails", h.db.scopes)
	}
}

func validExtractedLines() []domain.ExtractedRFQLine {
	return []domain.ExtractedRFQLine{
		{RequestedDescription: "cement", Quantity: decimal.RequireFromString("2.00")},
	}
}
