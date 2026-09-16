package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

/*
 * clientTransitions is the customer-action surface of the state machine, and it is deliberately
 * its own map rather than a reuse of the seller's. A customer answers a quote that was sent to
 * them and can do nothing else: they cannot send one, cannot reopen a closed one, and cannot move
 * a quote they were never given. Folding the two surfaces together is how a public token ends up
 * with a seller's reach.
 *
 * REQUEST_CHANGE is absent on purpose. Its ticket specifies that the comment is simultaneously a
 * client action and an inbound conversation message, joined by an explicit reference, and the
 * entities that answer for the second half are not built. Accepting it here would record the
 * action and drop the message.
 */
var clientTransitions = map[domain.ClientActionType]domain.QuoteStatus{
	domain.ClientActionAccept: domain.QuoteStatusAccepted,
	domain.ClientActionReject: domain.QuoteStatusRejected,
}

// clientActionRecorder is the persistence the customer's answer needs.
type clientActionRecorder interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewClientAction) (*domain.ClientAction, error)
}

// WithClientActions wires recording the answers a customer gives to a quote, and the seller lookup
// that lets the outcome reach whoever sent it.
func (s *QuoteDeliveryService) WithClientActions(
	actions clientActionRecorder, sellers quoteSellerReader,
) *QuoteDeliveryService {
	s.actions = actions
	s.sellers = sellers
	return s
}

/*
 * RecordClientAction is the customer answering the quote they were sent. The token is the whole
 * authority: there is no session behind it, so everything the write needs is read from the send it
 * resolves rather than taken from the caller.
 *
 * The action and the transition are one transaction. A recorded answer whose quote never moved
 * would leave the seller looking at a quote that is still merely sent, and a moved quote with no
 * action would lose why it moved.
 */
func (s *QuoteDeliveryService) RecordClientAction(
	ctx context.Context, token string, action domain.ClientActionType,
) (*domain.ClientQuoteOutcome, error) {
	target, known := clientTransitions[action]
	if !known {
		return nil, fmt.Errorf("%w: %q is not an answer a customer can give here",
			domain.ErrInvalidInput, action)
	}
	if s.actions == nil {
		return nil, domain.ErrNotConfigured
	}

	accountID, err := s.sends.GetAccountIDByPublicToken(ctx, s.db.CrossAccount(), token)
	if err != nil {
		return nil, err
	}

	var outcome domain.ClientQuoteOutcome
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			send, sendErr := s.sends.GetPublicByToken(ctx, q, accountID, token)
			if sendErr != nil {
				return sendErr
			}
			// An expired link is not an answer the quote should take: the prices it showed are
			// no longer the ones on offer.
			if send.ExpiresAt == nil || !s.now().Before(*send.ExpiresAt) {
				return domain.WithCode(domain.CodeQuoteSendExpired, domain.ErrConflict)
			}

			quote, quoteErr := s.quotes.GetByVersionID(ctx, q, accountID, send.VersionID)
			if quoteErr != nil {
				return quoteErr
			}
			if quote.CurrentStatus != domain.QuoteStatusSent {
				return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
			}

			if _, actionErr := s.actions.Create(ctx, q, accountID, domain.NewClientAction{
				VersionID:   send.VersionID,
				QuoteSendID: &send.ID,
				Type:        action,
			}); actionErr != nil {
				return actionErr
			}

			previous := quote.CurrentStatus
			moved, moveErr := s.quotes.UpdateStatus(ctx, q, accountID, quote.BranchID, quote.ID,
				previous, target)
			if moveErr != nil {
				return moveErr
			}
			// No user id: the customer is not one of ours, and attributing their answer to the
			// seller who sent it would read as the seller having closed the quote themselves.
			if _, changeErr := s.quotes.AppendStatusChange(ctx, q, accountID, quote.ID, &previous,
				target, nil); changeErr != nil {
				return changeErr
			}

			outcome = domain.ClientQuoteOutcome{QuoteID: moved.ID,
				Reference: domain.QuoteReference(moved.Number), Status: moved.CurrentStatus,
				Action: action, SellerID: moved.SellerID}
			return nil
		})
	if err != nil {
		return nil, err
	}

	s.notifySellerOfOutcome(ctx, accountID, outcome)
	return &outcome, nil
}
