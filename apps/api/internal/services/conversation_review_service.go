package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type conversationReviewQuotes interface {
	quoteVersionCloneRepository
	ClaimForEdit(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.Quote, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	GetPreviousVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, versionNumber int) (*domain.QuoteVersion, error)
	DeleteAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID, itemIDs []uuid.UUID) error
	DeleteItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	UpdateVersionTotal(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		total decimal.Decimal) (*domain.QuoteVersion, error)
}

type conversationReviewDiscounts interface {
	versionDiscountStore
	DeleteByVersion(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) error
}

type conversationReviewBatches interface {
	GetByQuote(ctx context.Context, q repository.Querier, accountID, quoteID,
		batchID uuid.UUID) (*domain.MessageBatch, error)
	Requeue(ctx context.Context, q repository.Querier, accountID, quoteID,
		batchID uuid.UUID) (*domain.MessageBatch, error)
}

// ConversationReviewService holds the seller's actions on the conversation handler's work: undoing
// its changes to a draft, and sending a batch that ran out of attempts back to the queue.
type ConversationReviewService struct {
	db        tenantTxRunner
	quotes    conversationReviewQuotes
	prices    branchPriceReader
	discounts conversationReviewDiscounts
	decisions handlerDecisionSettleStore
	batches   conversationReviewBatches
	sweep     *discountSweep
}

// NewConversationReviewService builds a ConversationReviewService.
func NewConversationReviewService(db tenantTxRunner, quotes conversationReviewQuotes,
	prices branchPriceReader, discounts conversationReviewDiscounts, promotions promotionReader,
	decisions handlerDecisionSettleStore, batches conversationReviewBatches,
) *ConversationReviewService {
	return &ConversationReviewService{db: db, quotes: quotes, prices: prices, discounts: discounts,
		decisions: decisions, batches: batches,
		sweep: &discountSweep{discounts: discounts, promotions: promotions, now: time.Now}}
}

// DiscardAIChanges resets a change-requested draft to the version the customer last received,
// repriced like a fresh draft, and records every AI change still pending on it as REJECTED. The
// quote stays CHANGE_REQUESTED. Returns the reset draft.
func (s *ConversationReviewService) DiscardAIChanges(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID,
) (*domain.QuoteVersion, error) {
	if err := requireBranch(tenant, "discarding the AI's changes"); err != nil {
		return nil, err
	}
	var draft *domain.QuoteVersion
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusChangeRequested {
			return domain.WithCode(domain.CodeNoAIChanges, domain.ErrConflict)
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if version.IsImmutable {
			return domain.WithCode(domain.CodeNoAIChanges, domain.ErrConflict)
		}
		traces, err := s.decisions.ListTracesByVersion(ctx, q, tenant.AccountID, version.ID)
		if err != nil {
			return err
		}
		rejected := make(map[uuid.UUID]domain.HandlerSellerDecision)
		for _, trace := range traces {
			if trace.SellerDecision == nil && trace.Outcome == domain.HandlerOutcomeApplied {
				rejected[trace.ID] = domain.HandlerDecisionRejected
			}
		}
		if len(rejected) == 0 {
			return domain.WithCode(domain.CodeNoAIChanges, domain.ErrConflict)
		}
		sent, err := s.quotes.GetPreviousVersion(ctx, q, tenant.AccountID, quote.BranchID, quote.ID,
			version.VersionNumber)
		if err != nil {
			return err
		}

		content, err := prepareVersionCopy(ctx, q, s.quotes, s.prices, tenant.AccountID,
			quote.BranchID, sent.ID)
		if err != nil {
			return err
		}
		current, err := s.quotes.ListItems(ctx, q, tenant.AccountID, version.ID)
		if err != nil {
			return err
		}
		if err := s.discounts.DeleteByVersion(ctx, q, tenant.AccountID, version.ID); err != nil {
			return err
		}
		if err := s.quotes.DeleteAlternativesByItemIDs(ctx, q, tenant.AccountID, version.ID,
			quoteItemIDs(current)); err != nil {
			return err
		}
		if err := s.quotes.DeleteItems(ctx, q, tenant.AccountID, version.ID,
			quoteItemIDs(current)); err != nil {
			return err
		}
		if _, err := insertVersionCopy(ctx, q, s.quotes, tenant.AccountID, version.ID,
			content); err != nil {
			return err
		}
		items, err := s.quotes.ListItems(ctx, q, tenant.AccountID, version.ID)
		if err != nil {
			return err
		}
		total, err := s.sweep.settle(ctx, q, tenant.AccountID, quote.BranchID, version.ID, items)
		if err != nil {
			return err
		}
		if draft, err = s.quotes.UpdateVersionTotal(ctx, q, tenant.AccountID, version.ID,
			total); err != nil {
			return err
		}
		_, err = s.decisions.Decide(ctx, q, tenant.AccountID, version.ID, tenant.UserID, rejected)
		return err
	})
	if err != nil {
		return nil, err
	}
	return draft, nil
}

// RetryBatch sends a batch that ran out of attempts back to the queue with a fresh count, while
// its quote is one the handler may still act on. Returns the requeued batch.
func (s *ConversationReviewService) RetryBatch(ctx context.Context, tenant domain.Tenant,
	quoteID, batchID uuid.UUID,
) (*domain.MessageBatch, error) {
	if err := requireBranch(tenant, "retrying a conversation batch"); err != nil {
		return nil, err
	}
	var requeued *domain.MessageBatch
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if _, err := s.batches.GetByQuote(ctx, q, tenant.AccountID, quote.ID, batchID); err != nil {
			return err
		}
		if domain.ValidateHandlerRun(quote.CurrentStatus, quote.ArchivedAt != nil) != nil {
			return domain.WithCode(domain.CodeBatchNotRetryable, domain.ErrConflict)
		}
		requeued, err = s.batches.Requeue(ctx, q, tenant.AccountID, quote.ID, batchID)
		if err != nil {
			return domain.WithCode(domain.CodeBatchNotRetryable, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return requeued, nil
}

type conversationBatchLister interface {
	ListByQuote(ctx context.Context, q repository.Querier, accountID,
		quoteID uuid.UUID) ([]domain.MessageBatch, error)
}

type conversationMessageLister interface {
	ListByQuote(ctx context.Context, q repository.Querier, accountID,
		quoteID uuid.UUID) ([]domain.QuoteMessage, error)
}

type conversationReviewLister interface {
	ListReviewsByQuote(ctx context.Context, q repository.Querier, accountID,
		quoteID uuid.UUID) (map[uuid.UUID]domain.HandlerDecisionReview, error)
}

type conversationReader struct {
	batches  conversationBatchLister
	messages conversationMessageLister
	reviews  conversationReviewLister
}

// load groups a quote's messages by batch with the handler's run on each, and sets the seller's
// sends beside them; nil until the customer has written.
func (r *conversationReader) load(ctx context.Context, q repository.Querier, accountID uuid.UUID,
	quote domain.Quote, version *domain.QuoteVersion,
) (*domain.Conversation, error) {
	all, err := r.messages.ListByQuote(ctx, q, accountID, quote.ID)
	if err != nil {
		return nil, err
	}
	messages := make([]domain.QuoteMessage, 0, len(all))
	sellerMessages := make([]domain.QuoteMessage, 0)
	for _, message := range all {
		if message.AuthorType == domain.MessageAuthorSeller {
			sellerMessages = append(sellerMessages, message)
		} else {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return nil, nil
	}
	batches, err := r.batches.ListByQuote(ctx, q, accountID, quote.ID)
	if err != nil {
		return nil, err
	}
	reviews, err := r.reviews.ListReviewsByQuote(ctx, q, accountID, quote.ID)
	if err != nil {
		return nil, err
	}

	byBatch := make(map[uuid.UUID][]domain.QuoteMessage, len(batches))
	conversation := &domain.Conversation{Batches: make([]domain.ConversationBatch, 0, len(batches)),
		Pending: make([]domain.QuoteMessage, 0), SellerMessages: sellerMessages}
	for _, message := range messages {
		if message.BatchID == nil {
			conversation.Pending = append(conversation.Pending, message)
			continue
		}
		byBatch[*message.BatchID] = append(byBatch[*message.BatchID], message)
	}
	handlerMayRun := domain.ValidateHandlerRun(quote.CurrentStatus, quote.ArchivedAt != nil) == nil
	for _, batch := range batches {
		if batch.Status == domain.MessageBatchStatusOpen {
			conversation.Pending = append(conversation.Pending, byBatch[batch.ID]...)
			continue
		}
		entry := domain.ConversationBatch{Batch: batch, Messages: byBatch[batch.ID]}
		if entry.Messages == nil {
			entry.Messages = []domain.QuoteMessage{}
		}
		if review, ok := reviews[batch.ID]; ok {
			entry.Decision = &review
			if version != nil && review.VersionID == version.ID && !version.IsImmutable &&
				review.SellerDecision == nil && review.Outcome == domain.HandlerOutcomeApplied {
				conversation.CanDiscardAIChanges = true
			}
		}
		entry.Retryable = handlerMayRun && batch.Status == domain.MessageBatchStatusFailed &&
			batch.FailureReason != nil &&
			*batch.FailureReason == domain.MessageBatchFailureAttemptsExhausted
		conversation.Batches = append(conversation.Batches, entry)
	}
	if quote.CurrentStatus != domain.QuoteStatusChangeRequested || quote.ArchivedAt != nil {
		conversation.CanDiscardAIChanges = false
	}
	return conversation, nil
}
