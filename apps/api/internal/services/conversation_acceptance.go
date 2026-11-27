package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type replyAcceptanceSends interface {
	ListByQuote(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) ([]domain.QuoteSend, error)
}

type replyAcceptanceActions interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewClientAction) (*domain.ClientAction, error)
}

type replyAcceptanceMarks interface {
	SetAcceptanceUnconfirmed(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, at *time.Time) (*domain.Quote, error)
}

type replyAcceptanceNotifier interface {
	NotifySellerOfOutcome(ctx context.Context, accountID uuid.UUID, outcome domain.ClientQuoteOutcome)
}

// replyAcceptance is what lets an acceptance read from a reply move the quote.
type replyAcceptance struct {
	policy   domain.ReplyAcceptancePolicy
	sends    replyAcceptanceSends
	actions  replyAcceptanceActions
	marks    replyAcceptanceMarks
	notifier replyAcceptanceNotifier
	now      func() time.Time
}

// ReplyAcceptancePolicy reads decision 91's thresholds from the conversation settings.
func ReplyAcceptancePolicy(cfg config.ConversationConfig) domain.ReplyAcceptancePolicy {
	return domain.ReplyAcceptancePolicy{
		AutomaticMinConfidencePercent: cfg.AutoAcceptMinConfidencePercent,
		ReviewMinConfidencePercent:    cfg.AcceptReviewMinConfidencePercent,
		MaxWords:                      cfg.AutoAcceptMaxWords,
	}
}

// WithReplyAcceptance lets an acceptance the backend's rules confirm move a SENT quote to
// ACCEPTED, on its own or pending the seller (decision 91). Without it every acceptance is only a
// signal for the seller.
func (h *BatchChangeHandler) WithReplyAcceptance(policy domain.ReplyAcceptancePolicy,
	sends replyAcceptanceSends, actions replyAcceptanceActions, marks replyAcceptanceMarks,
	notifier replyAcceptanceNotifier,
) *BatchChangeHandler {
	h.acceptance = &replyAcceptance{policy: policy, sends: sends, actions: actions, marks: marks,
		notifier: notifier, now: time.Now}
	return h
}

// acceptReply weighs an acceptance under the quote's lock and, when the rules allow it, accepts
// the sent version as the customer's portal click would: the status with no user, the customer
// action tied to the batch, and the seller told once the transaction commits.
func (h *BatchChangeHandler) acceptReply(ctx context.Context, q repository.Querier,
	batch domain.MessageBatch, quote domain.Quote, version domain.QuoteVersion,
	prepared preparedBatch, record *domain.HandlerProposalRecord,
) (*domain.ClientQuoteOutcome, error) {
	acceptance := h.acceptance
	facts := domain.ReplyAcceptanceFacts{Text: clientInput(prepared.messages),
		PartialOrAlternative: record.PartialOrAlternative,
		QuoteSent:            quote.CurrentStatus == domain.QuoteStatusSent && quote.ArchivedAt == nil}
	if record.DecisionConfidencePercent != nil {
		facts.ConfidencePercent = *record.DecisionConfidencePercent
	}
	for _, message := range prepared.messages {
		facts.PortalRequest = facts.PortalRequest || message.ClientActionID != nil
		facts.OtherLiveQuotes = max(facts.OtherLiveQuotes, message.OtherLiveQuotes)
	}
	sends, err := acceptance.sends.ListByQuote(ctx, q, quote.AccountID, quote.BranchID, quote.ID)
	if err != nil {
		return nil, err
	}
	now := acceptance.now().UTC()
	latest := latestLiveQuoteSend(sends)
	facts.SendLive = latest != nil && latest.VersionID == version.ID && latest.ExpiresAt != nil &&
		now.Before(*latest.ExpiresAt)
	alternatives, err := h.quotes.ListAlternativesByItemIDs(ctx, q, quote.AccountID,
		quoteItemIDs(prepared.items))
	if err != nil {
		return nil, err
	}
	for _, offered := range alternatives {
		for _, alternative := range offered {
			facts.AlternativesOffered = facts.AlternativesOffered || alternative.ApprovedBySeller
		}
	}
	account, err := h.accounts.GetByID(ctx, q, quote.AccountID)
	if err != nil {
		return nil, err
	}
	facts.AutoAcceptOn = account.WhatsAppAutoAccept

	tier, holds := acceptance.policy.Decide(facts)
	record.ReplyAcceptance, record.AcceptanceHolds = &tier, holds
	if tier == domain.ReplyAcceptanceSignal {
		return nil, nil
	}
	previous := quote.CurrentStatus
	accepted, err := h.quotes.UpdateStatus(ctx, q, quote.AccountID, quote.BranchID, quote.ID,
		previous, domain.QuoteStatusAccepted)
	if err != nil {
		return nil, err
	}
	if _, err := h.quotes.AppendStatusChange(ctx, q, quote.AccountID, quote.ID, &previous,
		domain.QuoteStatusAccepted, nil); err != nil {
		return nil, err
	}
	unconfirmed := tier == domain.ReplyAcceptanceUnconfirmed
	if unconfirmed {
		if _, err := acceptance.marks.SetAcceptanceUnconfirmed(ctx, q, quote.AccountID,
			quote.BranchID, quote.ID, &now); err != nil {
			return nil, err
		}
	}
	var comment *string
	if text := truncateHandlerText(facts.Text); text != "" {
		comment = &text
	}
	if _, err := acceptance.actions.Create(ctx, q, quote.AccountID, domain.NewClientAction{
		ID: uuid.New(), QuoteSendID: &latest.ID, VersionID: version.ID,
		Type: domain.ClientActionAccept, Comment: comment, MessageBatchID: &batch.ID,
	}); err != nil {
		return nil, err
	}
	return &domain.ClientQuoteOutcome{QuoteID: accepted.ID,
		Reference: string(domain.QuoteReference(accepted.Number)), Status: accepted.CurrentStatus,
		Action: domain.ClientActionAccept, SellerID: accepted.SellerID, ByReply: true,
		Unconfirmed: unconfirmed}, nil
}

// latestLiveQuoteSend is the newest send of the quote itself that reached the customer.
func latestLiveQuoteSend(sends []domain.QuoteSend) *domain.QuoteSend {
	var latest *domain.QuoteSend
	for i := range sends {
		send := &sends[i]
		if send.Purpose != domain.QuoteDeliveryPurposeQuote || send.SentAt == nil {
			continue
		}
		switch send.TrackingStatus {
		case domain.SendTrackingStatusSent, domain.SendTrackingStatusDelivered,
			domain.SendTrackingStatusViewed:
		default:
			continue
		}
		if latest == nil || send.SentAt.After(*latest.SentAt) {
			latest = send
		}
	}
	return latest
}
