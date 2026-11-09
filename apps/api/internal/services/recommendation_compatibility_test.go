package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
)

func TestCompatibleRecommendation_RequiresMatchingClassificationAndUnit(t *testing.T) {
	family, subgroup, other := uuid.New(), uuid.New(), uuid.New()
	unit, normalized, wrongUnit, blank := "bolsa", " BOLSA ", "m3", "  "
	base := domain.Product{FamilyID: &family, SubgroupID: &subgroup, Unit: &unit}
	cases := []struct {
		name      string
		candidate domain.Product
		want      bool
	}{
		{"same classification and normalized unit", domain.Product{FamilyID: &family, SubgroupID: &subgroup, Unit: &normalized}, true},
		{"different family", domain.Product{FamilyID: &other, SubgroupID: &subgroup, Unit: &unit}, false},
		{"different subgroup", domain.Product{FamilyID: &family, SubgroupID: &other, Unit: &unit}, false},
		{"different unit", domain.Product{FamilyID: &family, SubgroupID: &subgroup, Unit: &wrongUnit}, false},
		{"missing family", domain.Product{SubgroupID: &subgroup, Unit: &unit}, false},
		{"missing subgroup", domain.Product{FamilyID: &family, Unit: &unit}, false},
		{"missing unit", domain.Product{FamilyID: &family, SubgroupID: &subgroup}, false},
		{"blank unit", domain.Product{FamilyID: &family, SubgroupID: &subgroup, Unit: &blank}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compatibleRecommendation(base, tc.candidate); got != tc.want {
				t.Fatalf("compatibleRecommendation() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRecommendationPrice_RequiresStrictDirection(t *testing.T) {
	price := func(value string) decimal.NullDecimal {
		return decimal.NewNullDecimal(decimal.RequireFromString(value))
	}
	for _, tc := range []struct {
		name            string
		kind            domain.ProductAlternativeType
		base, candidate decimal.NullDecimal
		want            bool
	}{
		{"economy below", domain.ProductAlternativeEconomy, price("100"), price("99.99"), true},
		{"economy equal", domain.ProductAlternativeEconomy, price("100"), price("100"), false},
		{"economy above", domain.ProductAlternativeEconomy, price("100"), price("101"), false},
		{"premium above", domain.ProductAlternativePremium, price("100"), price("100.01"), true},
		{"premium equal", domain.ProductAlternativePremium, price("100"), price("100"), false},
		{"premium below", domain.ProductAlternativePremium, price("100"), price("99"), false},
		{"missing base", domain.ProductAlternativeEconomy, decimal.NullDecimal{}, price("99"), false},
		{"missing candidate", domain.ProductAlternativeEconomy, price("100"), decimal.NullDecimal{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validRecommendationPrice(tc.kind, tc.base, tc.candidate); got != tc.want {
				t.Fatalf("validRecommendationPrice() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProductService_RecommendationEndpointsRejectDifferentSubgroup(t *testing.T) {
	for _, set := range []bool{false, true} {
		h := newCatalogHarness(testProductID, testAlternativeID)
		alternative := h.products.known[testAlternativeID]
		other := uuid.New()
		alternative.SubgroupID = &other
		h.products.known[testAlternativeID] = alternative
		var err error
		if set {
			_, err = h.service.SetRecommendation(context.Background(), testTenant(), testProductID, testAlternativeID, domain.ProductAlternativeEconomy)
		} else {
			_, err = h.service.AddAlternative(context.Background(), testTenant(), testProductID, testAlternativeID, domain.ProductAlternativePremium)
		}
		if !errors.Is(err, domain.ErrInvalidInput) || len(h.alternatives.created) != 0 {
			t.Fatalf("set = %v: got error %v and %d writes", set, err, len(h.alternatives.created))
		}
	}
}

func TestQuoteService_SetRecommendation_RejectsDifferentSubgroup(t *testing.T) {
	baseID, alternativeID := uuid.New(), uuid.New()
	item := pricedLine(baseID, "10")
	f := newQuoteFixture([]domain.QuoteItem{item}, map[uuid.UUID]domain.BranchPrice{})
	f.quotes.quote.CurrentStatus = domain.QuoteStatusQuoted
	family, subgroup, other, unit := uuid.New(), uuid.New(), uuid.New(), "bolsa"
	f.service.WithRecommendationProducts(fakeRecommendationProducts{products: map[uuid.UUID]domain.Product{
		baseID:        {ID: baseID, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
		alternativeID: {ID: alternativeID, FamilyID: &family, SubgroupID: &other, Unit: &unit},
	}})
	_, err := f.service.SetRecommendationAlternative(context.Background(), f.tenant, f.quoteID, item.ID, alternativeID, domain.ProductAlternativeEconomy, false)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("SetRecommendationAlternative() = %v, want invalid input", err)
	}
}

func TestQuoteService_RecommendationPriceIsCheckedWhenSelectedAndApproved(t *testing.T) {
	for _, tc := range []struct {
		name           string
		kind           domain.ProductAlternativeType
		candidatePrice string
		wantCode       domain.ErrorCode
	}{
		{"equal economy", domain.ProductAlternativeEconomy, "100", domain.CodeRecommendationEconomyPriceInvalid},
		{"higher economy", domain.ProductAlternativeEconomy, "101", domain.CodeRecommendationEconomyPriceInvalid},
		{"equal premium", domain.ProductAlternativePremium, "100", domain.CodeRecommendationPremiumPriceInvalid},
		{"lower premium", domain.ProductAlternativePremium, "99", domain.CodeRecommendationPremiumPriceInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, candidate := uuid.New(), uuid.New()
			item := pricedLine(base, "10")
			item.UnitPriceSnapshot = decimal.NewNullDecimal(decimal.NewFromInt(100))
			f := newQuoteFixture([]domain.QuoteItem{item}, map[uuid.UUID]domain.BranchPrice{
				candidate: branchPrice(candidate, tc.candidatePrice, nil),
			})
			f.quotes.quote.CurrentStatus = domain.QuoteStatusQuoted
			_, err := f.service.SetRecommendationAlternative(context.Background(), f.tenant, f.quoteID, item.ID, candidate, tc.kind, false)
			if got := domain.CodeOf(err); got != tc.wantCode {
				t.Fatalf("selection code = %s, want %s (error %v)", got, tc.wantCode, err)
			}
			name, altID := "Alternative", uuid.New()
			f.quotes.alternatives = map[uuid.UUID][]domain.QuoteItemAlternative{item.ID: {{
				ID: altID, ProductID: &candidate, CanonicalName: &name, RecommendationType: &tc.kind,
				PriceSnapshot: decimal.NewNullDecimal(decimal.RequireFromString(tc.candidatePrice)),
			}}}
			_, err = f.service.ApproveAlternative(context.Background(), f.tenant, f.quoteID, item.ID, altID, true)
			if got := domain.CodeOf(err); got != tc.wantCode {
				t.Fatalf("approval code = %s, want %s (error %v)", got, tc.wantCode, err)
			}
		})
	}
}

func TestRepresentation_ExcludesRecommendationWhenBasePriceChanges(t *testing.T) {
	base, candidate, itemID, altID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	family, subgroup, unit := uuid.New(), uuid.New(), "bolsa"
	kind := domain.ProductAlternativeEconomy
	quotes := &fakePublicQuoteQuotes{}
	service := &QuoteRepresentationService{quotes: quotes, recommendationProducts: fakeRecommendationProducts{products: map[uuid.UUID]domain.Product{
		base:      {ID: base, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
		candidate: {ID: candidate, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
	}}}
	source := &domain.QuoteRepresentationSource{
		Items: []domain.QuoteItem{{ID: itemID, ProductID: &base, UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(90))}},
		Alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {{ID: altID, ProductID: &candidate, RecommendationType: &kind,
			PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(95)), ApprovedBySeller: true}}},
	}
	if err := service.excludeIncompatibleRecommendations(context.Background(), nil, uuid.New(), uuid.New(), source); err != nil {
		t.Fatal(err)
	}
	if len(source.Alternatives[itemID]) != 0 || len(quotes.unapproved) != 1 || quotes.unapproved[0] != altID {
		t.Fatalf("alternatives = %+v, unapproved = %v; want candidate excluded and unapproved", source.Alternatives[itemID], quotes.unapproved)
	}
}

func TestRepresentation_ExcludesIncompatibleApprovedRecommendations(t *testing.T) {
	baseID, validID, invalidID := uuid.New(), uuid.New(), uuid.New()
	itemID, validAltID, invalidAltID := uuid.New(), uuid.New(), uuid.New()
	family, subgroup, other, unit := uuid.New(), uuid.New(), uuid.New(), "bolsa"
	kind := domain.ProductAlternativeEconomy
	quotes := &fakePublicQuoteQuotes{}
	service := &QuoteRepresentationService{quotes: quotes, recommendationProducts: fakeRecommendationProducts{
		products: map[uuid.UUID]domain.Product{
			baseID:    {ID: baseID, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
			validID:   {ID: validID, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
			invalidID: {ID: invalidID, FamilyID: &family, SubgroupID: &other, Unit: &unit},
		},
	}}
	source := &domain.QuoteRepresentationSource{
		Items: []domain.QuoteItem{{ID: itemID, ProductID: &baseID, UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(100))}},
		Alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {
			{ID: validAltID, ProductID: &validID, RecommendationType: &kind, ApprovedBySeller: true, PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(90))},
			{ID: invalidAltID, ProductID: &invalidID, RecommendationType: &kind, ApprovedBySeller: true, PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(90))},
		}},
	}
	if err := service.excludeIncompatibleRecommendations(context.Background(), nil, uuid.New(), uuid.New(), source); err != nil {
		t.Fatal(err)
	}
	if len(source.Alternatives[itemID]) != 1 || source.Alternatives[itemID][0].ID != validAltID {
		t.Fatalf("retained options = %+v, want only compatible option", source.Alternatives[itemID])
	}
	if len(quotes.unapproved) != 1 || quotes.unapproved[0] != invalidAltID {
		t.Fatalf("unapproved = %v, want incompatible option", quotes.unapproved)
	}
}
