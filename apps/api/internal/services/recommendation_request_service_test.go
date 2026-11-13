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

func (f *fakeQuoteRepo) PromoteMatcherRecommendation(_ context.Context, _ repository.Querier,
	_, _, itemID, alternativeID, productID uuid.UUID, kind domain.ProductAlternativeType,
	price, minPrice decimal.NullDecimal,
) (*domain.QuoteItemAlternative, error) {
	for i := range f.alternatives[itemID] {
		offer := &f.alternatives[itemID][i]
		if offer.ID == alternativeID && offer.ProductID != nil && *offer.ProductID == productID {
			offer.RecommendationType = &kind
			offer.PriceSnapshot = price
			offer.MinPriceSnapshot = minPrice
			copy := *offer
			return &copy, nil
		}
	}
	return nil, domain.ErrNotFound
}

type recommendationRequestFixture struct {
	service    *RecommendationRequestService
	tenant     domain.Tenant
	quotes     *fakeQuoteRepo
	configured *fakeProductAlternatives
	prices     *fakeBranchPrices
	quoteID    uuid.UUID
	itemID     uuid.UUID
	baseID     uuid.UUID
	economyID  uuid.UUID
	premiumID  uuid.UUID
}

func newRecommendationRequestFixture() recommendationRequestFixture {
	accountID, branchID, quoteID, itemID, baseID, economyID, premiumID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	quotes := &fakeQuoteRepo{
		quote:        &domain.Quote{ID: quoteID, BranchID: branchID, CurrentStatus: domain.QuoteStatusQuoted},
		version:      &domain.QuoteVersion{ID: uuid.New(), Currency: "ARS"},
		items:        []domain.QuoteItem{{ID: itemID, ProductID: &baseID, UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(100))}},
		alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{},
	}
	configured := &fakeProductAlternatives{byType: map[domain.ProductAlternativeType]map[uuid.UUID][]domain.ProductAlternativeView{
		domain.ProductAlternativeEconomy: {baseID: {{Product: domain.Product{ID: economyID, CanonicalName: "Economy", IsActive: true}}}},
		domain.ProductAlternativePremium: {baseID: {{Product: domain.Product{ID: premiumID, CanonicalName: "Premium", IsActive: true}}}},
	}}
	prices := &fakeBranchPrices{prices: map[uuid.UUID]domain.BranchPrice{
		economyID: {Price: decimal.NewFromInt(80), Currency: "ARS"},
		premiumID: {Price: decimal.NewFromInt(120), Currency: "ARS"},
	}}
	return recommendationRequestFixture{service: NewRecommendationRequestService(&fakeQuoteDB{}, quotes, configured, prices),
		tenant: domain.Tenant{AccountID: accountID, BranchID: branchID}, quotes: quotes, configured: configured,
		prices: prices, quoteID: quoteID, itemID: itemID, baseID: baseID, economyID: economyID, premiumID: premiumID}
}

func TestRecommendationRequestService_Search_DoesNotMutateQuote(t *testing.T) {
	f := newRecommendationRequestFixture()
	proposals, err := f.service.Search(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 2 || proposals[0].ProductID != f.economyID || proposals[1].ProductID != f.premiumID {
		t.Fatalf("proposals = %+v, want both catalog slots", proposals)
	}
	if len(f.quotes.alternatives) != 0 || len(f.quotes.writtenTotals) != 0 {
		t.Fatal("search changed quote or seller decisions")
	}
	if len(f.prices.branches) != 1 || f.prices.branches[0] != f.tenant.BranchID {
		t.Fatalf("prices were read outside quote branch: %v", f.prices.branches)
	}
	if len(f.quotes.branchesAskedFor) != 1 || f.quotes.branchesAskedFor[0] != f.tenant.BranchID {
		t.Fatal("quote lookup was not scoped to active branch")
	}
}

func TestRecommendationRequestService_Search_FiltersUnavailablePricesAndOccupiedSlots(t *testing.T) {
	f := newRecommendationRequestFixture()
	f.prices.prices[f.economyID] = domain.BranchPrice{Price: decimal.NewFromInt(100), Currency: "ARS"}
	f.prices.prices[f.premiumID] = domain.BranchPrice{Price: decimal.NewFromInt(120), Currency: "USD"}
	proposals, err := f.service.Search(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(proposals) != 0 {
		t.Fatalf("proposals = %+v, err = %v, want none for equal price or other currency", proposals, err)
	}
	f.prices.prices[f.economyID] = domain.BranchPrice{Price: decimal.NewFromInt(80), Currency: "ARS"}
	f.prices.prices[f.premiumID] = domain.BranchPrice{Price: decimal.NewFromInt(120), Currency: "ARS"}
	economy := domain.ProductAlternativeEconomy
	f.quotes.alternatives[f.itemID] = []domain.QuoteItemAlternative{{RecommendationType: &economy, ApprovedBySeller: true}, {ProductID: &f.premiumID}}
	proposals, err = f.service.Search(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(proposals) != 0 {
		t.Fatalf("proposals = %+v, err = %v, want none for occupied slot or duplicate product", proposals, err)
	}
}

func TestRecommendationRequestService_AddIfVacant_RefusesASecondDecision(t *testing.T) {
	f := newRecommendationRequestFixture()
	first, err := f.service.AddIfVacant(context.Background(), f.tenant, f.quoteID, f.itemID, f.economyID, domain.ProductAlternativeEconomy)
	if err != nil || first == nil || first.ApprovedBySeller {
		t.Fatalf("first insert = %+v, %v", first, err)
	}
	_, err = f.service.AddIfVacant(context.Background(), f.tenant, f.quoteID, f.itemID, f.economyID, domain.ProductAlternativeEconomy)
	if !errors.Is(err, domain.ErrConflict) || len(f.quotes.alternatives[f.itemID]) != 1 || f.quotes.alternatives[f.itemID][0].ID != first.ID {
		t.Fatalf("duplicate insert = %v; existing offer changed", err)
	}
}

func TestRecommendationRequestService_AddIfVacant_PromotesUnapprovedMatcherCandidate(t *testing.T) {
	f := newRecommendationRequestFixture()
	matcherID := uuid.New()
	f.quotes.alternatives[f.itemID] = []domain.QuoteItemAlternative{{
		ID: matcherID, ProductID: &f.economyID, Type: domain.QuoteItemAlternativeTypeProduct,
		Origin: domain.QuoteItemAlternativeOriginAI, Rank: 4,
		ConfidenceScore: decimal.NewNullDecimal(decimal.RequireFromString("0.8")),
	}}
	proposals, err := f.service.Search(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(proposals) != 2 {
		t.Fatalf("Search() = %+v, %v; want the existing candidate proposed", proposals, err)
	}
	added, err := f.service.AddIfVacant(context.Background(), f.tenant, f.quoteID, f.itemID, f.economyID, domain.ProductAlternativeEconomy)
	if err != nil || added == nil || added.ID != matcherID || added.Origin != domain.QuoteItemAlternativeOriginAI ||
		added.Rank != 4 || added.RecommendationType == nil || *added.RecommendationType != domain.ProductAlternativeEconomy ||
		added.ApprovedBySeller || len(f.quotes.alternatives[f.itemID]) != 1 {
		t.Fatalf("matcher candidate was not preserved and promoted: %+v, %v", added, err)
	}
	approved := newRecommendationRequestFixture()
	approved.quotes.alternatives[approved.itemID] = []domain.QuoteItemAlternative{{
		ID: uuid.New(), ProductID: &approved.economyID, Type: domain.QuoteItemAlternativeTypeProduct,
		Origin: domain.QuoteItemAlternativeOriginAI, ApprovedBySeller: true,
	}}
	_, err = approved.service.AddIfVacant(context.Background(), approved.tenant, approved.quoteID, approved.itemID, approved.economyID, domain.ProductAlternativeEconomy)
	if !errors.Is(err, domain.ErrConflict) || approved.quotes.alternatives[approved.itemID][0].RecommendationType != nil {
		t.Fatalf("approved candidate changed: %v", err)
	}
}

func TestRecommendationRequestService_RefusesImmutableOrNonQuotedVersion(t *testing.T) {
	for _, state := range []string{"sent", "immutable"} {
		t.Run(state, func(t *testing.T) {
			f := newRecommendationRequestFixture()
			if state == "sent" {
				f.quotes.quote.CurrentStatus = domain.QuoteStatusSent
			} else {
				f.quotes.version.IsImmutable = true
			}
			_, err := f.service.Search(context.Background(), f.tenant, f.quoteID)
			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("Search() = %v, want conflict", err)
			}
			_, err = f.service.AddIfVacant(context.Background(), f.tenant, f.quoteID, f.itemID, f.economyID, domain.ProductAlternativeEconomy)
			if !errors.Is(err, domain.ErrConflict) || len(f.quotes.alternatives) != 0 {
				t.Fatalf("AddIfVacant() = %v, want conflict and no writes", err)
			}
		})
	}
}

func TestRecommendationRequestService_FillVacant_FillsOnlyEmptySlotsUnapproved(t *testing.T) {
	f := newRecommendationRequestFixture()
	filled, err := f.service.FillVacant(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(filled) != 2 {
		t.Fatalf("FillVacant() = %+v, %v; want both empty slots filled", filled, err)
	}
	for _, entry := range filled {
		if entry.ItemID != f.itemID || entry.Alternative.ApprovedBySeller || entry.Alternative.RecommendationType == nil {
			t.Fatalf("filled entry = %+v, want an unapproved slot of the item", entry)
		}
	}
	if len(f.quotes.alternatives[f.itemID]) != 2 {
		t.Fatalf("item offers = %+v, want the two added", f.quotes.alternatives[f.itemID])
	}

	// A second run finds both slots taken and changes nothing.
	again, err := f.service.FillVacant(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(again) != 0 || len(f.quotes.alternatives[f.itemID]) != 2 {
		t.Fatalf("second FillVacant() = %+v, %v; offers = %d", again, err, len(f.quotes.alternatives[f.itemID]))
	}
}

func TestRecommendationRequestService_FillVacant_KeepsTheSellersChoice(t *testing.T) {
	f := newRecommendationRequestFixture()
	economy := domain.ProductAlternativeEconomy
	chosen := uuid.New()
	f.quotes.alternatives[f.itemID] = []domain.QuoteItemAlternative{{
		ID: uuid.New(), ProductID: &chosen, RecommendationType: &economy, ApprovedBySeller: true,
	}}
	filled, err := f.service.FillVacant(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(filled) != 1 || filled[0].Alternative.ProductID == nil ||
		*filled[0].Alternative.ProductID != f.premiumID {
		t.Fatalf("FillVacant() = %+v, %v; want only the premium slot filled", filled, err)
	}
	offers := f.quotes.alternatives[f.itemID]
	if len(offers) != 2 || offers[0].ProductID == nil || *offers[0].ProductID != chosen || !offers[0].ApprovedBySeller {
		t.Fatalf("the seller's economy choice changed: %+v", offers)
	}
}

func TestRecommendationRequestService_FillVacant_RefusesAQuoteNotBeingPriced(t *testing.T) {
	f := newRecommendationRequestFixture()
	f.quotes.quote.CurrentStatus = domain.QuoteStatusSent
	_, err := f.service.FillVacant(context.Background(), f.tenant, f.quoteID)
	if !errors.Is(err, domain.ErrConflict) || len(f.quotes.alternatives[f.itemID]) != 0 {
		t.Fatalf("FillVacant() err = %v, offers = %+v; want a conflict and no change", err, f.quotes.alternatives)
	}
}

// currencyChangingPrices reports one product in another currency from the second read on.
type currencyChangingPrices struct {
	*fakeBranchPrices
	productID uuid.UUID
}

func (f *currencyChangingPrices) GetCurrentByProductIDs(
	ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, productIDs []uuid.UUID,
) (map[uuid.UUID]domain.BranchPrice, error) {
	prices, err := f.fakeBranchPrices.GetCurrentByProductIDs(ctx, q, accountID, branchID, productIDs)
	if err == nil && f.calls > 1 {
		price := prices[f.productID]
		price.Currency = "USD"
		prices[f.productID] = price
	}
	return prices, err
}

func TestRecommendationRequestService_FillVacant_SkipsAPriceThatChangedCurrency(t *testing.T) {
	f := newRecommendationRequestFixture()
	f.service.prices = &currencyChangingPrices{fakeBranchPrices: f.prices, productID: f.economyID}
	filled, err := f.service.FillVacant(context.Background(), f.tenant, f.quoteID)
	if err != nil || len(filled) != 1 || filled[0].Alternative.ProductID == nil ||
		*filled[0].Alternative.ProductID != f.premiumID {
		t.Fatalf("FillVacant() = %+v, %v; want only the premium slot, the economy price is now in USD", filled, err)
	}
}
