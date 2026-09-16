package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

/*
 * FoldAttachmentsIntoQuote runs the pipeline over the material an order's files carried and writes
 * the result only where nobody has reviewed it yet. It is the sweep's entry into the RFQ engine:
 * the inline intake extracts while the upload request is still open, and this extracts for files
 * that arrived on their own afterwards.
 *
 * The quote is read before the model runs, not after. A quote past DRAFT is not rewritten — the
 * file stays stored and surfaced, and folding it in is the seller's explicit action — and deciding
 * that up front costs one query instead of an extraction nobody may use.
 */
func (s *RFQService) FoldAttachmentsIntoQuote(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID, blocks []domain.Content,
	extractedText string,
) (domain.AttachmentFoldOutcome, error) {
	if s.extractor == nil || s.generations == nil {
		return "", fmt.Errorf("%w: the RFQ file pipeline is not fully wired", domain.ErrInvalidInput)
	}
	contentExtractor, ok := s.extractor.(domain.RFQContentExtractor)
	if !ok {
		return "", domain.ErrNotConfigured
	}
	rfq, quote, err := s.rfqWithQuote(ctx, tenant, rfqID)
	if err != nil {
		return "", err
	}
	if len(blocks) == 0 {
		s.failWithoutQuote(ctx, tenant, rfq, quote)
		return domain.AttachmentUnreadable, nil
	}
	// Archived counts as reviewed even at DRAFT: putting a quote away is something the seller
	// did, and a background write that reopens it is the sweep overruling them.
	if quote != nil && (quote.CurrentStatus != domain.QuoteStatusDraft || quote.ArchivedAt != nil) {
		return domain.AttachmentHeldForSeller, nil
	}

	pipelineCtx, cancel := context.WithTimeout(ctx, s.cfg.PipelineTimeout)
	defer cancel()
	extraction, items, alternatives, err := s.readMaterialsFromContent(pipelineCtx, tenant,
		contentExtractor, blocks, extractedText)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		s.failWithoutQuote(ctx, tenant, rfq, quote)
		return domain.AttachmentReadNoMaterials, nil
	}

	// No seller id anywhere below: a sweep is not a person, and attributing its write to the
	// seller who happens to own the order would read as them having drafted it themselves.
	if quote == nil {
		if _, err := s.persistGeneratedDraft(ctx, tenant, rfq, nil, extraction, items,
			alternatives); err != nil {
			return "", err
		}
		return domain.AttachmentFoldedIntoDraft, nil
	}
	if err := s.persistDraftVersion(ctx, tenant, quote, extraction, items, alternatives); err != nil {
		return "", err
	}
	return domain.AttachmentFoldedIntoDraft, nil
}

/*
 * failWithoutQuote closes out an order the sweep could get nothing out of. It only moves an order
 * that never got a quote: FAILED is what tells a seller to load that one by hand, and an order
 * that already has a quote keeps it, because a file nobody could read does not undo a quote that
 * already exists.
 */
func (s *RFQService) failWithoutQuote(
	ctx context.Context, tenant domain.Tenant, rfq *domain.RFQ, quote *domain.Quote,
) {
	if quote != nil || rfq.Status == domain.RFQStatusFailed {
		return
	}
	s.markRFQFailed(ctx, tenant, rfq, nil)
}

// rfqWithQuote reads the order and the quote it already has, if any. A missing quote is the
// ordinary case for an order whose pipeline never produced one, so it is not an error here.
func (s *RFQService) rfqWithQuote(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID,
) (*domain.RFQ, *domain.Quote, error) {
	var rfq *domain.RFQ
	var quote *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var rfqErr error
		rfq, rfqErr = s.rfqs.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, rfqID)
		if rfqErr != nil {
			return rfqErr
		}
		existing, quoteErr := s.quotes.GetByRFQID(ctx, q, tenant.AccountID, rfqID)
		if quoteErr != nil && !errors.Is(quoteErr, domain.ErrNotFound) {
			return quoteErr
		}
		quote = existing
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return rfq, quote, nil
}

/*
 * persistDraftVersion adds a draft version to a quote that already has one, which is what "feeds
 * the draft" means once an order has been through the pipeline before. The earlier version is
 * left where it is: a seller may have been editing it, and a new version keeps that work legible
 * instead of overwriting it.
 *
 * It never creates a second quote — the order already has one, and rfq → quote is one to one.
 */
func (s *RFQService) persistDraftVersion(
	ctx context.Context, tenant domain.Tenant, quote *domain.Quote,
	extraction *domain.RFQExtraction, items []domain.NewQuoteItem,
	alternatives []domain.NewQuoteItemAlternative,
) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, currentErr := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID,
			tenant.BranchID, quote.ID)
		if currentErr != nil {
			return currentErr
		}

		version, createVersionErr := s.quotes.CreateVersion(ctx, q, tenant.AccountID,
			domain.NewQuoteVersion{
				QuoteID:       quote.ID,
				VersionNumber: current.VersionNumber + 1,
				Currency:      current.Currency,
				Total:         decimal.Zero,
				IsImmutable:   false,
			})
		if createVersionErr != nil {
			return createVersionErr
		}

		createdItems, createItemsErr := s.quotes.CreateItems(ctx, q, tenant.AccountID,
			version.ID, items)
		if createItemsErr != nil {
			return createItemsErr
		}
		if _, candidatesErr := s.persistAlternatives(ctx, q, tenant.AccountID, createdItems,
			alternatives); candidatesErr != nil {
			return candidatesErr
		}
		generationItems, generationItemsErr := newQuoteAIGenerationItems(extraction.Lines, items)
		if generationItemsErr != nil {
			return generationItemsErr
		}
		if _, generationErr := s.generations.Create(ctx, q, tenant.AccountID,
			newQuoteAIGeneration(quote.ID, version.ID, extraction),
			generationItems); generationErr != nil {
			return generationErr
		}

		_, updateErr := s.quotes.UpdateCurrentVersion(ctx, q, tenant.AccountID, quote.ID,
			version.ID)
		return updateErr
	})
}
