package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// foldHarness is an rfqHarness with the order row the sweep reads before anything else.
func foldHarness(status domain.RFQStatus) *rfqHarness {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("20 bolsas de cemento", "20", "bolsa", "lo pidió así"),
	})
	h.rfqs.rfqRow = &domain.RFQ{ID: testRFQID, AccountID: testAccountID, BranchID: testBranchID,
		ChannelID: testChannelID, Status: status}
	return h
}

func materialBlocks() []domain.Content {
	return []domain.Content{domain.TextContent("20 bolsas de cemento")}
}

/*
 * The closed rule: the engine never rewrites a quote a seller has reviewed. From QUOTED on, a file
 * arriving late is stored and surfaced and folding it in is the seller's own action — and if the
 * quote is SENT, rewriting it would change what the client is reading at a public URL right now.
 *
 * The model must not even run: deciding this costs one query, and an extraction nobody may use is
 * paid for either way.
 */
func TestFoldAttachmentsIntoQuote_DoesNotRewriteAQuotePastReview(t *testing.T) {
	t.Parallel()
	for _, status := range []domain.QuoteStatus{domain.QuoteStatusQuoted, domain.QuoteStatusSent,
		domain.QuoteStatusAccepted, domain.QuoteStatusRejected,
		domain.QuoteStatusChangeRequested} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			h := foldHarness(domain.RFQStatusGenerated)
			h.quotes.quoteByID = &domain.Quote{ID: testQuoteID, RFQID: testRFQID,
				CurrentStatus: status}

			outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
				testRFQID, materialBlocks(), "20 bolsas de cemento")
			if err != nil {
				t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
			}
			if outcome != domain.AttachmentHeldForSeller {
				t.Errorf("outcome = %q, want the file held for the seller", outcome)
			}
			if h.extractor.calls != 0 {
				t.Errorf("extractor ran %d times, want none for a reviewed quote",
					h.extractor.calls)
			}
			if len(h.quotes.versions) != 0 {
				t.Errorf("wrote %d versions, want none", len(h.quotes.versions))
			}
		})
	}
}

// Archiving is something the seller did, so an archived draft is not fed either — a background
// write that reopened it would be the sweep overruling them.
func TestFoldAttachmentsIntoQuote_DoesNotWriteIntoAnArchivedDraft(t *testing.T) {
	t.Parallel()
	h := foldHarness(domain.RFQStatusGenerated)
	archived := time.Now()
	h.quotes.quoteByID = &domain.Quote{ID: testQuoteID, RFQID: testRFQID,
		CurrentStatus: domain.QuoteStatusDraft, ArchivedAt: &archived}

	outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
		testRFQID, materialBlocks(), "20 bolsas de cemento")
	if err != nil {
		t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
	}
	if outcome != domain.AttachmentHeldForSeller {
		t.Errorf("outcome = %q, want the file held for the seller", outcome)
	}
	if len(h.quotes.versions) != 0 {
		t.Errorf("wrote %d versions into an archived quote, want none", len(h.quotes.versions))
	}
}

// An order whose pipeline never produced a quote gets one from what its files carried — the
// ordinary case for a file that arrived after a failed ingest.
func TestFoldAttachmentsIntoQuote_DraftsAQuoteForAnOrderThatHasNone(t *testing.T) {
	t.Parallel()
	h := foldHarness(domain.RFQStatusReceived)

	outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
		testRFQID, materialBlocks(), "20 bolsas de cemento")
	if err != nil {
		t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
	}
	if outcome != domain.AttachmentFoldedIntoDraft {
		t.Errorf("outcome = %q, want the material folded into a draft", outcome)
	}
	if len(h.quotes.created) != 1 {
		t.Fatalf("created %d quotes, want one", len(h.quotes.created))
	}
	if h.quotes.created[0].CurrentStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status = %q, want DRAFT", h.quotes.created[0].CurrentStatus)
	}
	// A sweep is not a person, so nothing it writes is attributed to the seller who owns the order.
	if h.quotes.created[0].SellerID != nil {
		t.Errorf("seller = %v, want none recorded for a background sweep",
			h.quotes.created[0].SellerID)
	}
}

/*
 * An order that already has an unreviewed draft gets a NEW VERSION of the same quote, never a
 * second quote: rfq → quote is one to one and the database enforces it. The earlier version is
 * left where it is, because a seller may have been editing it.
 */
func TestFoldAttachmentsIntoQuote_AddsAVersionRatherThanASecondQuote(t *testing.T) {
	t.Parallel()
	h := foldHarness(domain.RFQStatusGenerated)
	h.quotes.quoteByID = &domain.Quote{ID: testQuoteID, RFQID: testRFQID,
		CurrentStatus: domain.QuoteStatusDraft}
	h.quotes.currentVersionData = &domain.QuoteVersion{ID: testVersionID, QuoteID: testQuoteID,
		VersionNumber: 1, Currency: domain.DefaultCurrency}

	outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
		testRFQID, materialBlocks(), "20 bolsas de cemento")
	if err != nil {
		t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
	}
	if outcome != domain.AttachmentFoldedIntoDraft {
		t.Errorf("outcome = %q, want the material folded into the draft", outcome)
	}
	if len(h.quotes.created) != 0 {
		t.Fatalf("created %d quotes, want none — the order already has one",
			len(h.quotes.created))
	}
	if len(h.quotes.versions) != 1 {
		t.Fatalf("created %d versions, want one", len(h.quotes.versions))
	}
	version := h.quotes.versions[0]
	if version.VersionNumber != 2 {
		t.Errorf("version number = %d, want the one after the current version",
			version.VersionNumber)
	}
	if version.IsImmutable {
		t.Error("new version is immutable, want a draft the seller can still edit")
	}
	if version.QuoteID != testQuoteID {
		t.Errorf("version quote = %s, want the order's existing quote %s", version.QuoteID,
			testQuoteID)
	}
}

/*
 * An order whose files were all unreadable never reaches the model, and the order stops looking
 * like it is still being worked: FAILED is what tells a seller to load it by hand.
 */
func TestFoldAttachmentsIntoQuote_FailsAnOrderItCouldReadNothingFor(t *testing.T) {
	t.Parallel()
	h := foldHarness(domain.RFQStatusReceived)

	outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
		testRFQID, nil, "")
	if err != nil {
		t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
	}
	if outcome != domain.AttachmentUnreadable {
		t.Errorf("outcome = %q, want the order reported unreadable", outcome)
	}
	if h.extractor.calls != 0 {
		t.Errorf("extractor ran %d times, want none with nothing to read", h.extractor.calls)
	}
	if len(h.rfqs.updatedStatus) != 1 || h.rfqs.updatedStatus[0] != domain.RFQStatusFailed {
		t.Errorf("rfq statuses = %v, want the order moved to FAILED", h.rfqs.updatedStatus)
	}
}

// A file nobody could read does not undo a quote that already exists, so an order that has one is
// left exactly where it was.
func TestFoldAttachmentsIntoQuote_LeavesAnOrderThatAlreadyHasAQuoteAlone(t *testing.T) {
	t.Parallel()
	h := foldHarness(domain.RFQStatusGenerated)
	h.quotes.quoteByID = &domain.Quote{ID: testQuoteID, RFQID: testRFQID,
		CurrentStatus: domain.QuoteStatusDraft}

	outcome, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(),
		testRFQID, nil, "")
	if err != nil {
		t.Fatalf("FoldAttachmentsIntoQuote() = %v, want no error", err)
	}
	if outcome != domain.AttachmentUnreadable {
		t.Errorf("outcome = %q, want the order reported unreadable", outcome)
	}
	if len(h.rfqs.updatedStatus) != 0 {
		t.Errorf("rfq statuses = %v, want the order left where it was", h.rfqs.updatedStatus)
	}
}

// The order is read before the model runs, and an order that does not belong to this tenant is
// refused there rather than after an extraction has been paid for.
func TestFoldAttachmentsIntoQuote_RefusesAnOrderItCannotRead(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	h.rfqs.rfqRowErr = domain.ErrNotFound

	if _, err := h.service.FoldAttachmentsIntoQuote(context.Background(), rfqTenant(), uuid.New(),
		materialBlocks(), "cemento"); err == nil {
		t.Fatal("FoldAttachmentsIntoQuote() = nil, want the order lookup refused")
	}
	if h.extractor.calls != 0 {
		t.Errorf("extractor ran %d times, want none", h.extractor.calls)
	}
}
