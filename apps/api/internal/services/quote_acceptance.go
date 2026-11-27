package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteAcceptanceStore interface {
	SetAcceptanceUnconfirmed(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, at *time.Time) (*domain.Quote, error)
	GetPreviousVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, versionNumber int) (*domain.QuoteVersion, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID,
		versionID uuid.UUID) (*domain.Quote, error)
	ListItems(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
	DeleteAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID, itemIDs []uuid.UUID) error
	DeleteItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	DeleteDraftVersion(ctx context.Context, q repository.Querier, accountID, quoteID,
		versionID uuid.UUID) error
}

type quoteAcceptanceActions interface {
	GetStandingAcceptance(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) (*domain.ClientAction, error)
	Revoke(ctx context.Context, q repository.Querier, accountID, actionID uuid.UUID) error
}

type quoteAcceptanceDrafts interface {
	DeleteByVersion(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) error
}

type quoteAcceptanceDecisions interface {
	SettleOnDiscard(ctx context.Context, q repository.Querier, accountID, draftID, keptID,
		userID uuid.UUID) error
}

// quoteReplyAcceptance is what the seller needs to settle an acceptance read from a reply.
type quoteReplyAcceptance struct {
	store     quoteAcceptanceStore
	actions   quoteAcceptanceActions
	sends     replyAcceptanceSends
	discounts quoteAcceptanceDrafts
	decisions quoteAcceptanceDecisions
	now       func() time.Time
}

// WithReplyAcceptance lets the seller confirm or undo an acceptance read from a customer's reply,
// and accept the sent version of a quote whose customer asked for changes.
func (s *QuoteService) WithReplyAcceptance(store quoteAcceptanceStore,
	actions quoteAcceptanceActions, sends replyAcceptanceSends, discounts quoteAcceptanceDrafts,
	decisions quoteAcceptanceDecisions,
) *QuoteService {
	s.acceptance = &quoteReplyAcceptance{store: store, actions: actions, sends: sends,
		discounts: discounts, decisions: decisions, now: time.Now}
	return s
}

// ConfirmAcceptance clears the pending confirmation of an acceptance read from a reply, which lets
// the sale be marked paid and invoiced. An acceptance with nothing pending is returned as it is.
func (s *QuoteService) ConfirmAcceptance(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID,
) (*domain.Quote, error) {
	if err := s.requireReplyAcceptance(tenant); err != nil {
		return nil, err
	}
	var confirmed *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.openSale(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		if quote.AcceptanceUnconfirmedAt == nil {
			confirmed = quote
			return nil
		}
		confirmed, err = s.acceptance.store.SetAcceptanceUnconfirmed(ctx, q, tenant.AccountID,
			quote.BranchID, quote.ID, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return confirmed, nil
}

// UndoAcceptance returns an acceptance read from a reply to SENT: only while the sale is neither
// paid, invoiced nor waived from invoicing, and the send it answered is still live. The customer
// action is revoked, so the portal takes an answer again.
func (s *QuoteService) UndoAcceptance(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID,
) (*domain.Quote, error) {
	if err := s.requireReplyAcceptance(tenant); err != nil {
		return nil, err
	}
	var undone *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.openSale(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		notUndoable := domain.WithCode(domain.CodeQuoteAcceptanceNotUndoable, domain.ErrConflict)
		if quote.PaidAt != nil || quote.InvoiceWaivedAt != nil || quote.CurrentVersionID == nil {
			return notUndoable
		}
		invoiced, err := s.quotes.HasLiveInvoice(ctx, q, tenant.AccountID, quote.BranchID,
			quote.ID, s.env)
		if err != nil {
			return err
		}
		if invoiced {
			return notUndoable
		}
		action, err := s.acceptance.actions.GetStandingAcceptance(ctx, q, tenant.AccountID,
			*quote.CurrentVersionID)
		if errors.Is(err, domain.ErrNotFound) || (err == nil && action.MessageBatchID == nil) {
			return notUndoable
		}
		if err != nil {
			return err
		}
		sends, err := s.acceptance.sends.ListByQuote(ctx, q, tenant.AccountID, quote.BranchID,
			quote.ID)
		if err != nil {
			return err
		}
		latest := latestLiveQuoteSend(sends)
		if latest == nil || latest.VersionID != *quote.CurrentVersionID || latest.ExpiresAt == nil ||
			!s.acceptance.now().Before(*latest.ExpiresAt) {
			return notUndoable
		}
		if err := s.acceptance.actions.Revoke(ctx, q, tenant.AccountID, action.ID); err != nil {
			return err
		}
		previous := quote.CurrentStatus
		if undone, err = s.quotes.UpdateStatus(ctx, q, tenant.AccountID, quote.BranchID, quote.ID,
			previous, domain.QuoteStatusSent); err != nil {
			return err
		}
		_, err = s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID, &previous,
			domain.QuoteStatusSent, &tenant.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return undone, nil
}

// AcceptSentVersion records that the customer of a CHANGE_REQUESTED quote took the version they
// were sent: the open draft is discarded, its pending runs settled, and the quote is ACCEPTED on the
// sent version by the seller.
func (s *QuoteService) AcceptSentVersion(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID,
) (*domain.Quote, error) {
	if err := s.requireReplyAcceptance(tenant); err != nil {
		return nil, err
	}
	var accepted *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusChangeRequested {
			return domain.WithCode(domain.CodeQuoteNotChangeRequested, domain.ErrConflict)
		}
		current, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID, quote.ID)
		if err != nil {
			return err
		}
		sent := current
		if !current.IsImmutable {
			if sent, err = s.acceptance.store.GetPreviousVersion(ctx, q, tenant.AccountID,
				quote.BranchID, quote.ID, current.VersionNumber); err != nil {
				return err
			}
			if err := s.discardDraft(ctx, q, tenant, *quote, *current, *sent); err != nil {
				return err
			}
		}
		previous := quote.CurrentStatus
		if accepted, err = s.quotes.UpdateStatus(ctx, q, tenant.AccountID, quote.BranchID, quote.ID,
			previous, domain.QuoteStatusAccepted); err != nil {
			return err
		}
		_, err = s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID, &previous,
			domain.QuoteStatusAccepted, &tenant.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return accepted, nil
}

// discardDraft points the quote back at the sent version and removes the draft cloned from it.
func (s *QuoteService) discardDraft(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, quote domain.Quote, draft, sent domain.QuoteVersion,
) error {
	store := s.acceptance.store
	if _, err := store.UpdateCurrentVersion(ctx, q, tenant.AccountID, quote.ID, sent.ID); err != nil {
		return err
	}
	if err := s.acceptance.decisions.SettleOnDiscard(ctx, q, tenant.AccountID, draft.ID, sent.ID,
		tenant.UserID); err != nil {
		return err
	}
	if err := s.acceptance.discounts.DeleteByVersion(ctx, q, tenant.AccountID, draft.ID); err != nil {
		return err
	}
	items, err := store.ListItems(ctx, q, tenant.AccountID, draft.ID)
	if err != nil {
		return err
	}
	ids := quoteItemIDs(items)
	if err := store.DeleteAlternativesByItemIDs(ctx, q, tenant.AccountID, draft.ID, ids); err != nil {
		return err
	}
	if err := store.DeleteItems(ctx, q, tenant.AccountID, draft.ID, ids); err != nil {
		return err
	}
	return store.DeleteDraftVersion(ctx, q, tenant.AccountID, quote.ID, draft.ID)
}

func (s *QuoteService) requireReplyAcceptance(tenant domain.Tenant) error {
	if s.acceptance == nil {
		return domain.ErrNotConfigured
	}
	return requireBranch(tenant, "settling an acceptance")
}
