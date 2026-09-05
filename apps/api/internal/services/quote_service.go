package services

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// quoteRepository is the quote persistence surface the lifecycle needs.
type quoteRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, from, to domain.QuoteStatus) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.QuoteVersion, error)
	UpdateVersionTotal(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, total decimal.Decimal) (*domain.QuoteVersion, error)
	ListItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID, itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	ApplyPricing(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, pricings []domain.QuoteItemPricing) error
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID, previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID) (*domain.QuoteStatusChange, error)
	Archive(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
	Unarchive(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
}

// branchPriceReader is the price-in-force surface valuation needs. One call carries every
// product on the quote: a lookup per line is the query-in-a-loop this exists to avoid.
type branchPriceReader interface {
	GetCurrentByProductIDs(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]domain.BranchPrice, error)
}

// QuoteService owns the quote lifecycle from the seller's review of the generated materials on.
type QuoteService struct {
	db     tenantTxRunner
	quotes quoteRepository
	prices branchPriceReader
	log    *slog.Logger
}

// NewQuoteService builds a QuoteService.
func NewQuoteService(
	db tenantTxRunner, quotes quoteRepository, prices branchPriceReader, log *slog.Logger,
) *QuoteService {
	if log == nil {
		log = slog.Default()
	}
	return &QuoteService{db: db, quotes: quotes, prices: prices, log: log}
}

// AcceptMaterials values a draft quote and moves it to QUOTED for human review. Returns
// domain.ErrConflict unless the quote is an unarchived DRAFT. See docs/technical/rfq-pipeline.md.
func (s *QuoteService) AcceptMaterials(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.PricedQuote, error) {
	if err := requireBranch(tenant, "a quote's prices"); err != nil {
		return nil, err
	}

	var priced domain.PricedQuote
	var unpricedProducts []uuid.UUID
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireMaterialsPendingAcceptance(*quote); err != nil {
			return err
		}

		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID,
			quote.ID)
		if err != nil {
			return err
		}
		items, err := s.quotes.ListItems(ctx, q, tenant.AccountID, version.ID)
		if err != nil {
			return err
		}
		// The candidates ride back with the lines: valuation does not change which line is
		// flagged, and the seller reviews the gaps and the choices on one screen.
		candidates, err := s.quotes.ListAlternativesByItemIDs(ctx, q, tenant.AccountID,
			quoteItemIDs(items))
		if err != nil {
			return err
		}

		// The quote's own branch, not the caller's selection: the price a line freezes belongs to
		// the branch the order arrived at. GetByID has already proved the two are the same.
		prices, err := s.prices.GetCurrentByProductIDs(ctx, q, tenant.AccountID, quote.BranchID,
			quoteItemProductIDs(items))
		if err != nil {
			return err
		}
		valuation, err := valueQuoteItems(items, prices)
		if err != nil {
			return err
		}
		unpricedProducts = valuation.unpricedProducts
		if err := s.quotes.ApplyPricing(ctx, q, tenant.AccountID, version.ID,
			valuation.pricings); err != nil {
			return err
		}
		version, err = s.quotes.UpdateVersionTotal(ctx, q, tenant.AccountID, version.ID,
			valuation.total)
		if err != nil {
			return err
		}

		// The version stays unfrozen. QUOTED and a frozen version are correlated but different
		// things: the seller still edits the draft, and freezing belongs to sending it.
		previousStatus := quote.CurrentStatus
		quote, err = s.quotes.UpdateStatus(ctx, q, tenant.AccountID, quote.BranchID, quote.ID,
			previousStatus, domain.QuoteStatusQuoted)
		if err != nil {
			// The status moved between the read and the write. Same refusal as the check above,
			// caught one statement later, so it reads the same to whoever clicked.
			if errors.Is(err, domain.ErrConflict) {
				return domain.WithCode(domain.CodeQuoteNotDraft, err)
			}
			return err
		}
		if _, err := s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID,
			&previousStatus, domain.QuoteStatusQuoted, &tenant.UserID); err != nil {
			return err
		}

		priced = domain.PricedQuote{
			Quote:           *quote,
			Version:         *version,
			Items:           valuation.items,
			UnpricedItemIDs: valuation.unpricedItems,
			Alternatives:    candidates,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Reported after the commit, not inside it: a warning about a quote whose transaction rolled
	// back would describe a valuation that never happened.
	if len(unpricedProducts) > 0 {
		s.log.WarnContext(ctx, "quote priced with lines the branch cannot price",
			slog.String("quote_id", priced.Quote.ID.String()),
			slog.String("branch_id", priced.Quote.BranchID.String()),
			slog.Int("lines", len(unpricedProducts)),
			slog.Any("product_ids", unpricedProducts))
	}
	return &priced, nil
}

// requireMaterialsPendingAcceptance is the state×intention check, run before anything is written.
// The two refusals share a status, so each carries the code that tells them apart.
func requireMaterialsPendingAcceptance(quote domain.Quote) error {
	if quote.ArchivedAt != nil {
		return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
	}
	if quote.CurrentStatus != domain.QuoteStatusDraft {
		return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
	}
	return nil
}

// sellerTransitions is the seller-action surface of the state machine, per docs/internal/domain/estados.md.
// It names the statuses a quote may move to from each current status. The Enviado→Cambio solicitado
// edge is included so the order can re-enter negotiation; the v2 draft that materializes it is the
// pipeline's own follow-on and not this transition's job.
var sellerTransitions = map[domain.QuoteStatus]map[domain.QuoteStatus]struct{}{
	domain.QuoteStatusQuoted: {
		domain.QuoteStatusSent: {},
	},
	domain.QuoteStatusSent: {
		domain.QuoteStatusAccepted:        {},
		domain.QuoteStatusRejected:        {},
		domain.QuoteStatusChangeRequested: {},
	},
	domain.QuoteStatusChangeRequested: {
		domain.QuoteStatusSent: {},
	},
}

// Transition moves a quote to the given status along a seller-action edge of the state machine. It
// refuses an already archived quote or one whose current status does not allow the move: the
// WriteUpdate predicate makes that refusal atomic, so a status read just before is not enough.
// Every step records the transition history.
func (s *QuoteService) Transition(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, to domain.QuoteStatus,
) (*domain.Quote, error) {
	if err := requireBranch(tenant, "a quote transition"); err != nil {
		return nil, err
	}

	var moved *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.ArchivedAt != nil {
			return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
		}
		targets, ok := sellerTransitions[quote.CurrentStatus]
		if !ok {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
		}
		if _, ok := targets[to]; !ok {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
		}

		previousStatus := quote.CurrentStatus
		moved, err = s.quotes.UpdateStatus(ctx, q, tenant.AccountID, tenant.BranchID, quoteID,
			previousStatus, to)
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.WithCode(domain.CodeQuoteNotDraft, err)
			}
			return err
		}
		if _, err := s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quoteID,
			&previousStatus, to, &tenant.UserID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

// Archive boxes a quote away without changing its status. Archived is an orthogonal flag, so this
// is not a transition and records no status change; it refuses an archived quote and a terminal
// one (ACCEPTED/REJECTED), which have no reason to be boxed away again.
func (s *QuoteService) Archive(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.Quote, error) {
	if err := requireBranch(tenant, "archiving a quote"); err != nil {
		return nil, err
	}

	var archived *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.ArchivedAt != nil {
			return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
		}
		if quote.CurrentStatus == domain.QuoteStatusAccepted || quote.CurrentStatus == domain.QuoteStatusRejected {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
		}
		var archiveErr error
		archived, archiveErr = s.quotes.Archive(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		return archiveErr
	})
	if err != nil {
		return nil, err
	}
	return archived, nil
}

// Unarchive brings a boxed-away quote back to the list.
func (s *QuoteService) Unarchive(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.Quote, error) {
	if err := requireBranch(tenant, "unarchiving a quote"); err != nil {
		return nil, err
	}

	var restored *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		restored, err = s.quotes.Unarchive(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}
