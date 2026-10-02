package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

/*
 * quoteSellerReader resolves the seller a quote belongs to, so the answer can be addressed to them.
 *
 * The seller hears about it by mail, through the sender this service already holds: the product has
 * no in-app notification surface, and mail is the one channel it has, with its own templates, rate
 * limiting and delivery ledger. This is deliberately not a new mechanism.
 */
type quoteSellerReader interface {
	GetByID(ctx context.Context, q repository.Querier, accountID,
		id uuid.UUID) (*domain.AppUser, error)
}

/*
 * notifySellerOfOutcome tells the quote's seller what their customer decided.
 *
 * It runs after the transaction has committed and never returns an error: the customer's answer is
 * recorded and the quote has moved, and failing their request because the seller's mail bounced
 * would undo a decision the customer already made. The queue shows the new status either way, so a
 * missed mail costs promptness rather than the fact.
 */
func (s *QuoteDeliveryService) notifySellerOfOutcome(ctx context.Context, accountID uuid.UUID,
	outcome domain.ClientQuoteOutcome) {
	if s.sellers == nil || s.email == nil {
		return
	}

	// A quote nobody has taken has nobody to tell. The status change still stands.
	if outcome.SellerID == nil {
		return
	}

	subject, heading, body := quoteOutcomeCopy(outcome)
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			seller, sellerErr := s.sellers.GetByID(ctx, q, accountID, *outcome.SellerID)
			if sellerErr != nil {
				return sellerErr
			}
			return s.email.Send(ctx, OutboundMail{AccountID: accountID, UserID: &seller.ID,
				QuoteID: &outcome.QuoteID, Event: quoteOutcomeEvent(outcome.Action),
				To: seller.Email, Subject: subject, Heading: heading, Paragraphs: []string{body}})
		})
	if err != nil {
		s.log.ErrorContext(ctx, "could not tell the seller their quote was answered",
			slog.String("quote_id", outcome.QuoteID.String()),
			slog.String("action", string(outcome.Action)), slog.Any("error", err))
	}
}

func quoteOutcomeEvent(action domain.ClientActionType) domain.NotificationEvent {
	if action == domain.ClientActionAccept {
		return domain.NotificationEventQuoteAccepted
	}
	return domain.NotificationEventQuoteRejected
}
