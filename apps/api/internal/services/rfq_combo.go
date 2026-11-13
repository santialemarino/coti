package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type comboSuggestionReader interface {
	ListSoldAtBranch(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) ([]domain.Combo, error)
}

// WithCombos wires the catalog combos a priced quote is offered to complete.
func (s *RFQService) WithCombos(combos comboSuggestionReader) *RFQService {
	s.combos = combos
	return s
}

// ComboSuggestions lists the combos the quote's branch sells that the priced quote holds part of,
// with the products still missing. It writes nothing.
func (s *RFQService) ComboSuggestions(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) ([]domain.ComboSuggestion, error) {
	if err := requireBranch(tenant, "combo suggestions"); err != nil {
		return nil, err
	}
	suggestions := []domain.ComboSuggestion{}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted || quote.ArchivedAt != nil || s.combos == nil {
			return nil
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID)
		if err != nil || version.IsImmutable {
			return err
		}
		suggestions, err = s.findComboSuggestions(ctx, q, tenant.AccountID, quote.BranchID, version.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return suggestions, nil
}

// AddComboItems adds a suggested combo's missing products as priced lines, in one transaction,
// and settles the total and promotions once. The seller pressing it is the approval.
func (s *RFQService) AddComboItems(
	ctx context.Context, tenant domain.Tenant, quoteID, comboID uuid.UUID,
) ([]domain.QuoteItem, error) {
	if err := requireBranch(tenant, "adding a combo"); err != nil {
		return nil, err
	}
	var added []domain.QuoteItem
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireEditableLines(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted || s.combos == nil {
			return domain.ErrConflict
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if version.IsImmutable {
			return domain.ErrImmutable
		}
		suggestions, err := s.findComboSuggestions(ctx, q, tenant.AccountID, quote.BranchID, version.ID)
		if err != nil {
			return err
		}
		var suggestion *domain.ComboSuggestion
		for i := range suggestions {
			if suggestions[i].ComboID == comboID {
				suggestion = &suggestions[i]
			}
		}
		if suggestion == nil {
			return domain.ErrConflict
		}
		missingIDs := make([]uuid.UUID, len(suggestion.Missing))
		for i, item := range suggestion.Missing {
			missingIDs[i] = item.ProductID
		}
		prices, err := s.prices.GetCurrentByProductIDs(ctx, q, tenant.AccountID, quote.BranchID, missingIDs)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(suggestion.Missing))
		for _, item := range suggestion.Missing {
			productID := item.ProductID
			created, err := s.quotes.CreateSingleItem(ctx, q, tenant.AccountID, version.ID, domain.QuoteItemCreate{
				ProductID: &productID, RequestedDescription: item.ProductName,
				Quantity: item.Quantity, Unit: item.ProductUnit,
			})
			if err != nil {
				return err
			}
			price := prices[productID]
			update := domain.QuoteItemUpdate{ProductID: &productID, UnitPriceSnapshot: &price.Price}
			if price.MinPrice.Valid {
				update.MinPriceSnapshot = &price.MinPrice.Decimal
			}
			if _, err := s.quotes.UpdateItem(ctx, q, tenant.AccountID, version.ID, created.ID, update); err != nil {
				return err
			}
			ids = append(ids, created.ID)
		}
		if err := s.recalculateVersionTotal(ctx, q, tenant, version.ID); err != nil {
			return err
		}
		for _, id := range ids {
			item, err := s.quotes.GetItem(ctx, q, tenant.AccountID, version.ID, id)
			if err != nil {
				return err
			}
			added = append(added, *item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

func (s *RFQService) findComboSuggestions(
	ctx context.Context, q repository.Querier, accountID, branchID, versionID uuid.UUID,
) ([]domain.ComboSuggestion, error) {
	combos, err := s.combos.ListSoldAtBranch(ctx, q, accountID, branchID)
	if err != nil || len(combos) == 0 {
		return []domain.ComboSuggestion{}, err
	}
	items, err := s.quotes.ListItems(ctx, q, accountID, versionID)
	if err != nil {
		return nil, err
	}
	productIDs := make([]uuid.UUID, 0)
	for _, combo := range combos {
		for _, item := range combo.Items {
			productIDs = append(productIDs, item.ProductID)
		}
	}
	prices, err := s.prices.GetCurrentByProductIDs(ctx, q, accountID, branchID, productIDs)
	if err != nil {
		return nil, err
	}
	return comboSuggestions(combos, items, prices), nil
}

/*
 * comboSuggestions offers a combo when the quote holds at least one of its products and lacks at
 * least one. A combo whose missing product is inactive or has no price at the branch is left out:
 * adding it would put an unpriced line in a priced quote.
 */
func comboSuggestions(
	combos []domain.Combo, items []domain.QuoteItem, prices map[uuid.UUID]domain.BranchPrice,
) []domain.ComboSuggestion {
	inQuote := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if item.ProductID != nil {
			inQuote[*item.ProductID] = true
		}
	}
	suggestions := []domain.ComboSuggestion{}
	for _, combo := range combos {
		suggestion := domain.ComboSuggestion{ComboID: combo.ID, Name: combo.Name, Description: combo.Description}
		sellable := true
		for _, item := range combo.Items {
			if inQuote[item.ProductID] {
				suggestion.Present = append(suggestion.Present, item)
				continue
			}
			price, priced := prices[item.ProductID]
			if !item.ProductIsActive || !priced || !price.Price.GreaterThan(decimal.Zero) {
				sellable = false
			}
			suggestion.Missing = append(suggestion.Missing, item)
		}
		if sellable && len(suggestion.Present) > 0 && len(suggestion.Missing) > 0 {
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions
}
