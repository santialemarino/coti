package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// quoteRepository is the quote persistence surface the lifecycle needs.
type quoteRepository interface {
	quoteVersionCloneRepository
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID, id uuid.UUID) (*domain.Quote, error)
	ClaimForEdit(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, from, to domain.QuoteStatus) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.QuoteVersion, error)
	UpdateVersionValuation(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		total decimal.Decimal, currency string) (*domain.QuoteVersion, error)
	GetAlternative(ctx context.Context, q repository.Querier, accountID, versionID, itemID, alternativeID uuid.UUID) (*domain.QuoteItemAlternative, error)
	ApplyPricing(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, pricings []domain.QuoteItemPricing) error
	ApplyAlternativePricing(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, pricings []domain.QuoteItemAlternativePricing) error
	UpdateAlternativeApproval(ctx context.Context, q repository.Querier, accountID, versionID, itemID, alternativeID uuid.UUID, approved bool) (*domain.QuoteItemAlternative, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID, previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID) (*domain.QuoteStatusChange, error)
	Archive(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
	Unarchive(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID) (*domain.Quote, error)
	HasLiveInvoice(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, env domain.ARCAEnvironment) (bool, error)
	SetPaid(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, userID *uuid.UUID) (*domain.Quote, error)
	SetInvoiceWaived(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID, userID *uuid.UUID) (*domain.Quote, error)
}

// invoicingSwitch reads whether the account invoices with Coti.
type invoicingSwitch interface {
	InvoicingEnabled(ctx context.Context, q repository.Querier, accountID uuid.UUID) (bool, error)
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
	env    domain.ARCAEnvironment // where this server invoices; only an invoice issued there closes a sale.
	log    *slog.Logger
	// sweep settles the version's discounts once its lines are priced; nil prices without them.
	sweep *discountSweep
	// carry starts a reactivated draft from the discounts the customer saw; nil without the engine.
	carry *draftCarry
	// invoicing is the account switch; nil reads as off, so no sale is waived from an invoice.
	invoicing invoicingSwitch
}

// WithInvoicingSwitch wires the account switch «No requiere factura» depends on.
func (s *QuoteService) WithInvoicingSwitch(invoicing invoicingSwitch) *QuoteService {
	s.invoicing = invoicing
	return s
}

// WithDiscountEngine wires the discount sweep, so accepting the materials also earns the promotions
// the priced lines qualify for.
func (s *QuoteService) WithDiscountEngine(discounts interface {
	versionDiscountStore
	draftDiscountCopier
}, promotions promotionReader, totals draftTotalWriter) *QuoteService {
	s.sweep = &discountSweep{discounts: discounts, promotions: promotions, now: time.Now}
	s.carry = &draftCarry{discounts: discounts, sweep: s.sweep, totals: totals}
	return s
}

// NewQuoteService builds a QuoteService.
func NewQuoteService(
	db tenantTxRunner, quotes quoteRepository, prices branchPriceReader, env domain.ARCAEnvironment, log *slog.Logger,
) *QuoteService {
	if log == nil {
		log = slog.Default()
	}
	return &QuoteService{db: db, quotes: quotes, prices: prices, env: env, log: log}
}

// AcceptMaterials values a draft quote and moves it to QUOTED for human review. Returns
// domain.ErrConflict unless the quote is an unarchived DRAFT or CHANGE_REQUESTED.
func (s *QuoteService) AcceptMaterials(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.PricedQuote, error) {
	if err := requireBranch(tenant, "a quote's prices"); err != nil {
		return nil, err
	}

	var priced domain.PricedQuote
	var unpricedProducts []uuid.UUID
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
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
		if version.IsImmutable {
			return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
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
			quoteProductIDs(items, candidates))
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
		alternativePricings, err := valueQuoteAlternatives(candidates, prices, valuation.currency)
		if err != nil {
			return err
		}
		if err := s.quotes.ApplyAlternativePricing(ctx, q, tenant.AccountID, version.ID,
			alternativePricings); err != nil {
			return err
		}
		applyAlternativePricing(candidates, alternativePricings)
		total := valuation.total
		if s.sweep != nil {
			total, err = s.sweep.settle(ctx, q, tenant.AccountID, quote.BranchID, version.ID,
				valuation.items)
			if err != nil {
				return err
			}
		}
		version, err = s.quotes.UpdateVersionValuation(ctx, q, tenant.AccountID, version.ID,
			total, valuation.currency)
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

// ApproveAlternative records a seller decision on a priced candidate of the current version.
func (s *QuoteService) ApproveAlternative(
	ctx context.Context, tenant domain.Tenant, quoteID, itemID, alternativeID uuid.UUID,
	approved bool,
) (*domain.QuoteItemAlternative, error) {
	if err := requireBranch(tenant, "a quote alternative"); err != nil {
		return nil, err
	}
	var result *domain.QuoteItemAlternative
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted {
			return domain.ErrConflict
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if version.IsImmutable {
			return domain.ErrConflict
		}
		alternative, err := s.quotes.GetAlternative(ctx, q, tenant.AccountID, version.ID,
			itemID, alternativeID)
		if err != nil {
			return err
		}
		if approved && (!alternative.PriceSnapshot.Valid || alternative.CanonicalName == nil) {
			return fmt.Errorf("%w: an approved alternative needs a frozen price and catalog identity",
				domain.ErrInvalidInput)
		}
		result, err = s.quotes.UpdateAlternativeApproval(ctx, q, tenant.AccountID, version.ID,
			itemID, alternativeID, approved)
		return err
	})
	return result, err
}

// requireMaterialsPendingAcceptance is the state×intention check, run before anything is written.
// The two refusals share a status, so each carries the code that tells them apart.
func requireMaterialsPendingAcceptance(quote domain.Quote) error {
	if quote.ArchivedAt != nil {
		return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
	}
	if quote.CurrentStatus != domain.QuoteStatusDraft &&
		quote.CurrentStatus != domain.QuoteStatusChangeRequested {
		return domain.WithCode(domain.CodeQuoteNotDraft, domain.ErrConflict)
	}
	return nil
}

// sellerTransitions is the seller-action surface of the state machine, per docs/internal/domain/estados.md.
// It names seller-only status changes; a change request needs a message and a new draft in the
// customer action transaction before AcceptMaterials can price it.
var sellerTransitions = map[domain.QuoteStatus]map[domain.QuoteStatus]struct{}{
	domain.QuoteStatusQuoted: {
		domain.QuoteStatusSent: {},
	},
	domain.QuoteStatusSent: {
		domain.QuoteStatusAccepted: {},
		domain.QuoteStatusRejected: {},
	},
}

// Reactivate reopens an accepted or rejected quote. RESEND keeps the frozen version and returns
// to QUOTED; EDIT creates a repriced mutable version and returns to CHANGE_REQUESTED.
func (s *QuoteService) Reactivate(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, mode domain.QuoteReactivationMode,
) (*domain.Quote, error) {
	if err := requireBranch(tenant, "a quote reactivation"); err != nil {
		return nil, err
	}
	if mode != domain.QuoteReactivationResend && mode != domain.QuoteReactivationEdit {
		return nil, fmt.Errorf("%w: unknown quote reactivation mode %q", domain.ErrInvalidInput,
			mode)
	}

	var moved *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID)
		if err != nil {
			return err
		}
		if quote.ArchivedAt != nil {
			return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
		}
		if quote.CurrentStatus != domain.QuoteStatusAccepted &&
			quote.CurrentStatus != domain.QuoteStatusRejected {
			return domain.WithCode(domain.CodeQuoteNotReactivatable, domain.ErrConflict)
		}
		// An invoiced sale is a fiscal fact: reopening it would let a second invoice go out for it.
		invoiced, err := s.quotes.HasLiveInvoice(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID, s.env)
		if err != nil {
			return err
		}
		if invoiced {
			return domain.WithCode(domain.CodeQuoteAlreadyInvoiced, domain.ErrConflict)
		}
		// Paid closes the sale for everyone; reopening it starts by taking the mark back.
		if quote.PaidAt != nil {
			return domain.WithCode(domain.CodeQuoteAlreadyPaid, domain.ErrConflict)
		}

		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID,
			quote.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.WithCode(domain.CodeQuoteNotReactivatable, domain.ErrConflict)
			}
			return err
		}
		if !version.IsImmutable {
			return domain.WithCode(domain.CodeQuoteNotReactivatable, domain.ErrConflict)
		}

		target := domain.QuoteStatusQuoted
		if mode == domain.QuoteReactivationEdit {
			authorID := tenant.UserID
			if _, err := cloneQuoteVersionForEditing(ctx, q, s.quotes, s.prices,
				tenant.AccountID, quote.BranchID, quote.ID, *version, &authorID, nil,
				s.carry); err != nil {
				return err
			}
			target = domain.QuoteStatusChangeRequested
		}

		previousStatus := quote.CurrentStatus
		moved, err = s.quotes.UpdateStatus(ctx, q, tenant.AccountID, quote.BranchID, quote.ID,
			previousStatus, target)
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.WithCode(domain.CodeQuoteNotReactivatable, err)
			}
			return err
		}
		if _, err := s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID,
			&previousStatus, target, &tenant.UserID); err != nil {
			return err
		}
		// A reopened sale is a new sale to close, so «No requiere factura» does not carry over.
		if moved.InvoiceWaivedAt != nil {
			moved, err = s.quotes.SetInvoiceWaived(ctx, q, tenant.AccountID, quote.BranchID, quote.ID, nil)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
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
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
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
// is not a transition and records no status change.
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

// MarkPaid records «Cobrado» on an accepted, unarchived sale, with who and when. It is a fact, not a
// transition: the status, its history and what the AI reads stay as they are. Marking a paid sale
// again keeps the first mark.
func (s *QuoteService) MarkPaid(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error) {
	return s.setPaid(ctx, tenant, quoteID, &tenant.UserID)
}

// UnmarkPaid takes «Cobrado» back from an accepted, unarchived sale.
func (s *QuoteService) UnmarkPaid(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error) {
	return s.setPaid(ctx, tenant, quoteID, nil)
}

// WaiveInvoice records «No requiere factura» on an accepted, unarchived sale, taking it out of the
// invoicing wait. It is refused while invoicing is off or the sale has a live invoice.
func (s *QuoteService) WaiveInvoice(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error) {
	if err := requireBranch(tenant, "closing a sale"); err != nil {
		return nil, err
	}
	var waived *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.openSale(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		enabled := false
		if s.invoicing != nil {
			if enabled, err = s.invoicing.InvoicingEnabled(ctx, q, tenant.AccountID); err != nil {
				return err
			}
		}
		if !enabled {
			return errAccountInvoicingOff
		}
		// The quote row is locked, so an issue cannot slip a pending invoice in after this read.
		invoiced, err := s.quotes.HasLiveInvoice(ctx, q, tenant.AccountID, quote.BranchID, quote.ID, s.env)
		if err != nil {
			return err
		}
		if invoiced {
			return domain.WithCode(domain.CodeQuoteAlreadyInvoiced, domain.ErrConflict)
		}
		waived, err = s.quotes.SetInvoiceWaived(ctx, q, tenant.AccountID, quote.BranchID, quote.ID, &tenant.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return waived, nil
}

// RestoreInvoiceWait undoes «No requiere factura», so the sale waits for its invoice again.
func (s *QuoteService) RestoreInvoiceWait(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Quote, error) {
	if err := requireBranch(tenant, "closing a sale"); err != nil {
		return nil, err
	}
	var restored *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.openSale(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		restored, err = s.quotes.SetInvoiceWaived(ctx, q, tenant.AccountID, quote.BranchID, quote.ID, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}

func (s *QuoteService) setPaid(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID, userID *uuid.UUID,
) (*domain.Quote, error) {
	if err := requireBranch(tenant, "closing a sale"); err != nil {
		return nil, err
	}
	var marked *domain.Quote
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.openSale(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		marked, err = s.quotes.SetPaid(ctx, q, tenant.AccountID, quote.BranchID, quote.ID, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return marked, nil
}

// openSale locks a quote and refuses it unless it is an accepted sale still in the list: closing
// facts are written only there.
func (s *QuoteService) openSale(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.Quote, error) {
	quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return nil, err
	}
	if quote.ArchivedAt != nil {
		return nil, domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
	}
	if quote.CurrentStatus != domain.QuoteStatusAccepted {
		return nil, domain.WithCode(domain.CodeQuoteNotAccepted, domain.ErrConflict)
	}
	return quote, nil
}
