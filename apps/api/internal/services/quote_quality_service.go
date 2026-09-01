package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// WholeQuoteEvaluatorVersion identifies the exact deterministic labeling rules in this file.
const WholeQuoteEvaluatorVersion = "whole-quote-v1"

// QuoteQualityEvaluator is the integration point the future send flow calls after it commits the
// frozen SENT version. The operation is idempotent, so a failed post-send attempt can be retried.
type QuoteQualityEvaluator interface {
	EvaluateFinalQuote(ctx context.Context, tenant domain.Tenant, quoteID,
		finalVersionID uuid.UUID) (*domain.QuoteQualityEvaluation, error)
}

type quoteQualityRepository interface {
	GetGenerationByQuoteID(ctx context.Context, q repository.Querier, accountID,
		quoteID uuid.UUID) (*domain.QuoteAIGeneration, error)
	ListGenerationItems(ctx context.Context, q repository.Querier, accountID,
		generationID uuid.UUID) ([]domain.QuoteAIGenerationItem, error)
	GetFinalVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteQualityFinalVersion, error)
	ListFinalItems(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
	CreateEvaluation(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteQualityEvaluation, differences []domain.NewQuoteQualityDifference,
	) (*domain.QuoteQualityEvaluation, error)
}

// QuoteQualityService labels a frozen seller-approved quote against its original AI proposal.
type QuoteQualityService struct {
	db      tenantTxRunner
	quality quoteQualityRepository
}

// NewQuoteQualityService builds a QuoteQualityService.
func NewQuoteQualityService(db tenantTxRunner, quality quoteQualityRepository) *QuoteQualityService {
	return &QuoteQualityService{db: db, quality: quality}
}

// EvaluateFinalQuote appends the ground-truth label after sending has frozen the version. The
// future send service must call it only after the QUOTED to SENT transaction commits successfully.
func (s *QuoteQualityService) EvaluateFinalQuote(
	ctx context.Context, tenant domain.Tenant, quoteID, finalVersionID uuid.UUID,
) (*domain.QuoteQualityEvaluation, error) {
	if err := requireBranch(tenant, "a final quote evaluation"); err != nil {
		return nil, err
	}
	if quoteID == uuid.Nil || finalVersionID == uuid.Nil {
		return nil, fmt.Errorf("%w: quote and final version ids are required",
			domain.ErrInvalidInput)
	}
	if s.quality == nil {
		return nil, fmt.Errorf("%w: quote quality persistence is not wired",
			domain.ErrInvalidInput)
	}

	var evaluation *domain.QuoteQualityEvaluation
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		generation, err := s.quality.GetGenerationByQuoteID(ctx, q, tenant.AccountID, quoteID)
		if err != nil {
			return err
		}
		proposed, err := s.quality.ListGenerationItems(ctx, q, tenant.AccountID, generation.ID)
		if err != nil {
			return err
		}
		final, err := s.quality.GetFinalVersion(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID, finalVersionID)
		if err != nil {
			return err
		}
		final.Items, err = s.quality.ListFinalItems(ctx, q, tenant.AccountID, finalVersionID)
		if err != nil {
			return err
		}

		label, differences := evaluateWholeQuote(generation.ID, proposed, *final)
		evaluation, err = s.quality.CreateEvaluation(ctx, q, tenant.AccountID, label, differences)
		return err
	})
	if err != nil {
		return nil, err
	}
	return evaluation, nil
}

var _ QuoteQualityEvaluator = (*QuoteQualityService)(nil)

func evaluateWholeQuote(
	generationID uuid.UUID, proposed []domain.QuoteAIGenerationItem,
	final domain.QuoteQualityFinalVersion,
) (domain.NewQuoteQualityEvaluation, []domain.NewQuoteQualityDifference) {
	differences := make([]domain.NewQuoteQualityDifference, 0)
	usedFinal := make(map[uuid.UUID]bool, len(final.Items))
	finalByID := make(map[uuid.UUID]domain.QuoteItem, len(final.Items))
	for _, item := range final.Items {
		finalByID[item.ID] = item
	}

	allEquivalent := true
	for _, generated := range proposed {
		item, found := finalByID[generated.SourceQuoteItemID]
		if found && usedFinal[item.ID] {
			found = false
		}
		if !found {
			item, found = findEquivalentFinalItem(generated, final.Items, usedFinal)
		}
		if !found {
			allEquivalent = false
			generationItemID := generated.ID
			differences = append(differences, domain.NewQuoteQualityDifference{
				Kind:             domain.QuoteQualityDifferenceItemRemoved,
				GenerationItemID: &generationItemID,
				ExpectedValue:    stringPointer(generated.RequestedDescription),
			})
			continue
		}
		usedFinal[item.ID] = true
		itemDifferences := compareBillableItem(generated, item)
		if len(itemDifferences) > 0 {
			allEquivalent = false
			differences = append(differences, itemDifferences...)
		}
	}
	for _, item := range final.Items {
		if usedFinal[item.ID] {
			continue
		}
		allEquivalent = false
		finalItemID := item.ID
		differences = append(differences, domain.NewQuoteQualityDifference{
			Kind: domain.QuoteQualityDifferenceItemAdded, FinalQuoteItemID: &finalItemID,
			ActualValue: stringPointer(item.RequestedDescription),
		})
	}

	allMatched, allPriced, allSubtotals := true, true, true
	subtotalSum := decimal.Zero
	for _, item := range final.Items {
		itemID := item.ID
		if item.MatchStatus != domain.ItemMatchStatusMatched {
			allMatched = false
			differences = append(differences, domain.NewQuoteQualityDifference{
				Kind: domain.QuoteQualityDifferenceUnresolvedMatch, FinalQuoteItemID: &itemID,
				Field: stringPointer("match_status"), ActualValue: stringPointer(string(item.MatchStatus)),
			})
		}
		if item.ProductID == nil || !item.UnitPriceSnapshot.Valid || !item.Subtotal.Valid {
			allPriced = false
			differences = append(differences, domain.NewQuoteQualityDifference{
				Kind: domain.QuoteQualityDifferenceMissingPrice, FinalQuoteItemID: &itemID,
			})
			allSubtotals = false
			continue
		}
		expected := item.Quantity.Mul(item.UnitPriceSnapshot.Decimal).Round(domain.MoneyScale)
		if !expected.Equal(item.Subtotal.Decimal) {
			allSubtotals = false
			differences = append(differences, domain.NewQuoteQualityDifference{
				Kind: domain.QuoteQualityDifferenceInvalidSubtotal, FinalQuoteItemID: &itemID,
				Field: stringPointer("subtotal"), ExpectedValue: stringPointer(expected.StringFixed(2)),
				ActualValue: stringPointer(item.Subtotal.Decimal.StringFixed(2)),
			})
		}
		subtotalSum = subtotalSum.Add(item.Subtotal.Decimal)
	}
	expectedTotal := subtotalSum.Sub(final.DiscountTotal).Round(domain.MoneyScale)
	totalValid := expectedTotal.Equal(final.Version.Total)
	if !totalValid {
		differences = append(differences, domain.NewQuoteQualityDifference{
			Kind: domain.QuoteQualityDifferenceInvalidTotal, Field: stringPointer("total"),
			ExpectedValue: stringPointer(expectedTotal.StringFixed(2)),
			ActualValue:   stringPointer(final.Version.Total.StringFixed(2)),
		})
	}

	sameCount := len(proposed) == len(final.Items)
	wholeCorrect := sameCount && allEquivalent && allMatched && allPriced && allSubtotals &&
		totalValid
	return domain.NewQuoteQualityEvaluation{
		GenerationID: generationID, FinalQuoteVersionID: final.Version.ID,
		EvaluatorVersion: WholeQuoteEvaluatorVersion, WholeQuoteCorrect: wholeCorrect,
		SameItemCount: sameCount, AllItemsEquivalent: allEquivalent,
		AllItemsMatched: allMatched, AllItemsPriced: allPriced,
		AllSubtotalsValid: allSubtotals, TotalValid: totalValid,
	}, differences
}

func findEquivalentFinalItem(
	generated domain.QuoteAIGenerationItem, final []domain.QuoteItem, used map[uuid.UUID]bool,
) (domain.QuoteItem, bool) {
	for _, item := range final {
		if !used[item.ID] && billableItemEquivalent(generated, item) {
			return item, true
		}
	}
	return domain.QuoteItem{}, false
}

func billableItemEquivalent(generated domain.QuoteAIGenerationItem, final domain.QuoteItem) bool {
	return uuidPointersEqual(generated.ProductID, final.ProductID) &&
		generated.Quantity.Equal(final.Quantity) && optionalUnitsEqual(generated.Unit, final.Unit)
}

func compareBillableItem(
	generated domain.QuoteAIGenerationItem, final domain.QuoteItem,
) []domain.NewQuoteQualityDifference {
	var differences []domain.NewQuoteQualityDifference
	appendDifference := func(field string, proposed, actual *string) {
		generationItemID, finalItemID := generated.ID, final.ID
		differences = append(differences, domain.NewQuoteQualityDifference{
			Kind:             domain.QuoteQualityDifferenceFieldChanged,
			GenerationItemID: &generationItemID, FinalQuoteItemID: &finalItemID,
			Field: &field, ExpectedValue: proposed, ActualValue: actual,
		})
	}
	if !uuidPointersEqual(generated.ProductID, final.ProductID) {
		appendDifference("product_id", uuidStringPointer(generated.ProductID),
			uuidStringPointer(final.ProductID))
	}
	if !generated.Quantity.Equal(final.Quantity) {
		appendDifference("quantity", stringPointer(generated.Quantity.StringFixed(2)),
			stringPointer(final.Quantity.StringFixed(2)))
	}
	if !optionalUnitsEqual(generated.Unit, final.Unit) {
		appendDifference("unit", generated.Unit, final.Unit)
	}
	return differences
}

func uuidPointersEqual(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func optionalUnitsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return strings.EqualFold(strings.TrimSpace(*left), strings.TrimSpace(*right))
}

func uuidStringPointer(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	return stringPointer(id.String())
}

func stringPointer(value string) *string {
	return &value
}
