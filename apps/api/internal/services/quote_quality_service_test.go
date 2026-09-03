package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestCorrectionPatterns_LearnsOnlyInterpretationAndCatalogCorrections(t *testing.T) {
	t.Parallel()
	generatedID, finalID := uuid.New(), uuid.New()
	oldProduct, correctedProduct := uuid.New(), uuid.New()
	unit := "bolsa"
	proposed := []domain.QuoteAIGenerationItem{{
		ID: generatedID, RequestedDescription: "cemento fuerte", ProductID: &oldProduct,
		Quantity: decimal.NewFromInt(5), Unit: &unit,
	}}
	final := []domain.QuoteItem{{
		ID: finalID, RequestedDescription: "cemento fuerte", ProductID: &correctedProduct,
		Quantity: decimal.NewFromInt(10), Unit: &unit,
	}}
	productField, quantityField := "product_id", "quantity"
	differences := []domain.NewQuoteQualityDifference{
		{Kind: domain.QuoteQualityDifferenceFieldChanged, Field: &productField,
			GenerationItemID: &generatedID, FinalQuoteItemID: &finalID},
		{Kind: domain.QuoteQualityDifferenceFieldChanged, Field: &quantityField,
			GenerationItemID: &generatedID, FinalQuoteItemID: &finalID},
		{Kind: domain.QuoteQualityDifferenceInvalidTotal},
	}

	patterns := correctionPatterns("mandame 10 de cemento fuerte", proposed, final, differences)
	if len(patterns) != 2 {
		t.Fatalf("correctionPatterns() returned %d patterns, want interpretation and catalog", len(patterns))
	}
	if patterns[0].Kind != domain.QuoteCorrectionMemoryCatalog ||
		patterns[0].ProductID == nil || *patterns[0].ProductID != correctedProduct {
		t.Errorf("catalog pattern = %+v, want corrected product", patterns[0])
	}
	if patterns[1].Kind != domain.QuoteCorrectionMemoryInterpretation ||
		len(patterns[1].CorrectedItems) != 1 ||
		!patterns[1].CorrectedItems[0].Quantity.Equal(decimal.NewFromInt(10)) {
		t.Errorf("interpretation pattern = %+v, want seller-approved quantity", patterns[1])
	}
}

func TestCorrectionPatterns_SentAsProposedTeachesNothing(t *testing.T) {
	t.Parallel()
	if got := correctionPatterns("cemento", nil, nil, nil); len(got) != 0 {
		t.Fatalf("correctionPatterns() returned %d patterns, want none", len(got))
	}
}

func TestEvaluateWholeQuote_AcceptsEquivalentBillableContentAndValidMoney(t *testing.T) {
	t.Parallel()
	generationID, versionID := uuid.New(), uuid.New()
	firstItemID, secondItemID := uuid.New(), uuid.New()
	firstProduct, secondProduct := uuid.New(), uuid.New()
	bag, bagUpper, unit := "bolsa", "BOLSA", "unidad"
	proposed := []domain.QuoteAIGenerationItem{
		generationItem(firstItemID, firstProduct, "2", &bag),
		generationItem(secondItemID, secondProduct, "3", &unit),
	}
	final := domain.QuoteQualityFinalVersion{
		Version: domain.QuoteVersion{ID: versionID, Total: decimal.RequireFromString("150.00")},
		Items: []domain.QuoteItem{
			finalItem(secondItemID, secondProduct, "3", &unit, "20", "60"),
			finalItem(firstItemID, firstProduct, "2", &bagUpper, "50", "100"),
		},
		DiscountTotal: decimal.RequireFromString("10"),
	}
	final.Items[1].RequestedDescription = "Descripción editorial corregida"

	got, differences := evaluateWholeQuote(generationID, proposed, final)

	if !got.WholeQuoteCorrect || !got.SameItemCount || !got.AllItemsEquivalent ||
		!got.AllItemsMatched || !got.AllItemsPriced || !got.AllSubtotalsValid ||
		!got.TotalValid {
		t.Errorf("evaluation = %+v, want every whole-quote condition true", got)
	}
	if len(differences) != 0 {
		t.Errorf("differences = %+v, want none for equivalent billable content", differences)
	}
}

func TestEvaluateWholeQuote_ExplainsEveryMaterialAndMoneyCorrection(t *testing.T) {
	t.Parallel()
	generationID, versionID, itemID := uuid.New(), uuid.New(), uuid.New()
	proposedProduct, finalProduct := uuid.New(), uuid.New()
	bag, kilogram := "bolsa", "kg"
	proposed := []domain.QuoteAIGenerationItem{
		generationItem(itemID, proposedProduct, "2", &bag),
	}
	finalItem := finalItem(itemID, finalProduct, "3", &kilogram, "10", "999")
	finalItem.MatchStatus = domain.ItemMatchStatusAmbiguous
	final := domain.QuoteQualityFinalVersion{
		Version: domain.QuoteVersion{ID: versionID, Total: decimal.RequireFromString("1000")},
		Items:   []domain.QuoteItem{finalItem},
	}

	got, differences := evaluateWholeQuote(generationID, proposed, final)

	if got.WholeQuoteCorrect || got.AllItemsEquivalent || got.AllItemsMatched ||
		got.AllSubtotalsValid || got.TotalValid {
		t.Errorf("evaluation = %+v, want the corrected quote to fail those conditions", got)
	}
	if !got.SameItemCount || !got.AllItemsPriced {
		t.Errorf("evaluation = %+v, want count and price presence to remain true", got)
	}
	wantKinds := map[domain.QuoteQualityDifferenceKind]int{
		domain.QuoteQualityDifferenceFieldChanged:    3,
		domain.QuoteQualityDifferenceUnresolvedMatch: 1,
		domain.QuoteQualityDifferenceInvalidSubtotal: 1,
		domain.QuoteQualityDifferenceInvalidTotal:    1,
	}
	gotKinds := make(map[domain.QuoteQualityDifferenceKind]int)
	for _, difference := range differences {
		gotKinds[difference.Kind]++
	}
	for kind, count := range wantKinds {
		if gotKinds[kind] != count {
			t.Errorf("difference %s count = %d, want %d; all differences: %+v",
				kind, gotKinds[kind], count, differences)
		}
	}
}

func TestEvaluateWholeQuote_ReportsRemovedAddedAndUnpricedLines(t *testing.T) {
	t.Parallel()
	proposedItemID, finalItemID := uuid.New(), uuid.New()
	product, unit := uuid.New(), "unidad"
	proposed := []domain.QuoteAIGenerationItem{
		generationItem(proposedItemID, product, "1", &unit),
	}
	added := finalItem(finalItemID, product, "2", &unit, "1", "2")
	added.ProductID = nil
	added.UnitPriceSnapshot = decimal.NullDecimal{}
	added.Subtotal = decimal.NullDecimal{}
	added.MatchStatus = domain.ItemMatchStatusNoMatch
	final := domain.QuoteQualityFinalVersion{
		Version: domain.QuoteVersion{ID: uuid.New(), Total: decimal.Zero},
		Items:   []domain.QuoteItem{added},
	}

	got, differences := evaluateWholeQuote(uuid.New(), proposed, final)

	if got.WholeQuoteCorrect || got.AllItemsEquivalent || got.AllItemsMatched ||
		got.AllItemsPriced || got.AllSubtotalsValid {
		t.Errorf("evaluation = %+v, want removed, added and unpriced failures", got)
	}
	for _, kind := range []domain.QuoteQualityDifferenceKind{
		domain.QuoteQualityDifferenceItemRemoved,
		domain.QuoteQualityDifferenceItemAdded,
		domain.QuoteQualityDifferenceUnresolvedMatch,
		domain.QuoteQualityDifferenceMissingPrice,
	} {
		if !hasQualityDifference(differences, kind) {
			t.Errorf("differences = %+v, want %s", differences, kind)
		}
	}
}

type fakeQuoteQualityRepository struct {
	generation  *domain.QuoteAIGeneration
	proposed    []domain.QuoteAIGenerationItem
	final       *domain.QuoteQualityFinalVersion
	created     []domain.NewQuoteQualityEvaluation
	differences [][]domain.NewQuoteQualityDifference
	finalErr    error
}

func (f *fakeQuoteQualityRepository) GetGenerationByQuoteID(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (*domain.QuoteAIGeneration, error) {
	return f.generation, nil
}

func (f *fakeQuoteQualityRepository) ListGenerationItems(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) ([]domain.QuoteAIGenerationItem, error) {
	return f.proposed, nil
}

func (f *fakeQuoteQualityRepository) GetFinalVersion(
	_ context.Context, _ repository.Querier, _, _, _, _ uuid.UUID,
) (*domain.QuoteQualityFinalVersion, error) {
	if f.finalErr != nil {
		return nil, f.finalErr
	}
	copy := *f.final
	copy.Items = nil
	return &copy, nil
}

func (f *fakeQuoteQualityRepository) ListFinalItems(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) ([]domain.QuoteItem, error) {
	return f.final.Items, nil
}

func (f *fakeQuoteQualityRepository) CreateEvaluation(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
	in domain.NewQuoteQualityEvaluation, differences []domain.NewQuoteQualityDifference,
) (*domain.QuoteQualityEvaluation, error) {
	f.created = append(f.created, in)
	f.differences = append(f.differences, differences)
	return &domain.QuoteQualityEvaluation{
		ID: uuid.New(), AccountID: accountID, GenerationID: in.GenerationID,
		FinalQuoteVersionID: in.FinalQuoteVersionID, EvaluatorVersion: in.EvaluatorVersion,
		WholeQuoteCorrect: in.WholeQuoteCorrect,
	}, nil
}

func TestQuoteQualityService_EvaluateFinalQuote_LabelsInsideOneTenantTransaction(t *testing.T) {
	t.Parallel()
	accountID, branchID, quoteID := uuid.New(), uuid.New(), uuid.New()
	generationID, versionID, itemID, productID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	unit := "bolsa"
	db := &fakeQuoteDB{}
	repo := &fakeQuoteQualityRepository{
		generation: &domain.QuoteAIGeneration{ID: generationID, QuoteID: quoteID},
		proposed: []domain.QuoteAIGenerationItem{
			generationItem(itemID, productID, "2", &unit),
		},
		final: &domain.QuoteQualityFinalVersion{
			Version: domain.QuoteVersion{ID: versionID, Total: decimal.RequireFromString("100")},
			Items: []domain.QuoteItem{
				finalItem(itemID, productID, "2", &unit, "50", "100"),
			},
		},
	}
	service := NewQuoteQualityService(db, repo)

	evaluation, err := service.EvaluateFinalQuote(context.Background(), domain.Tenant{
		AccountID: accountID, BranchID: branchID, UserID: uuid.New(),
	}, quoteID, versionID)

	if err != nil {
		t.Fatalf("EvaluateFinalQuote returned %v", err)
	}
	if !evaluation.WholeQuoteCorrect || len(repo.created) != 1 || len(repo.differences[0]) != 0 {
		t.Errorf("evaluation = %+v writes=%+v differences=%+v, want one correct label",
			evaluation, repo.created, repo.differences)
	}
	if db.transactions != 1 || len(db.scopes) != 1 || db.scopes[0] != accountID {
		t.Errorf("transactions/scopes = %d/%v, want one transaction for %s",
			db.transactions, db.scopes, accountID)
	}
}

func TestQuoteQualityService_EvaluateFinalQuote_RefusesBeforeTheCompletedSendExists(t *testing.T) {
	t.Parallel()
	db := &fakeQuoteDB{}
	repo := &fakeQuoteQualityRepository{
		generation: &domain.QuoteAIGeneration{ID: uuid.New(), QuoteID: uuid.New()},
		finalErr:   domain.ErrNotFound,
	}
	service := NewQuoteQualityService(db, repo)

	_, err := service.EvaluateFinalQuote(context.Background(), domain.Tenant{
		AccountID: uuid.New(), BranchID: uuid.New(), UserID: uuid.New(),
	}, uuid.New(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("EvaluateFinalQuote returned %v, want ErrNotFound before the completed send", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("created evaluations %v, want none before the send outcome exists", repo.created)
	}
}

func generationItem(
	sourceItemID, productID uuid.UUID, quantity string, unit *string,
) domain.QuoteAIGenerationItem {
	return domain.QuoteAIGenerationItem{
		ID: uuid.New(), SourceQuoteItemID: sourceItemID, ProductID: &productID,
		RequestedDescription: "pedido original", Quantity: decimal.RequireFromString(quantity),
		Unit: unit, MatchStatus: domain.ItemMatchStatusMatched,
	}
}

func finalItem(
	id, productID uuid.UUID, quantity string, unit *string, unitPrice, subtotal string,
) domain.QuoteItem {
	return domain.QuoteItem{
		ID: id, ProductID: &productID, RequestedDescription: "pedido original",
		Quantity: decimal.RequireFromString(quantity), Unit: unit,
		UnitPriceSnapshot: decimal.NewNullDecimal(decimal.RequireFromString(unitPrice)),
		Subtotal:          decimal.NewNullDecimal(decimal.RequireFromString(subtotal)),
		MatchStatus:       domain.ItemMatchStatusMatched,
	}
}

func hasQualityDifference(
	differences []domain.NewQuoteQualityDifference, kind domain.QuoteQualityDifferenceKind,
) bool {
	for _, difference := range differences {
		if difference.Kind == kind {
			return true
		}
	}
	return false
}
