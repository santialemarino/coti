package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeRFQPriceCatalog struct {
	groups  map[uuid.UUID][]domain.RFQPriceCandidate
	err     error
	calls   int
	account uuid.UUID
	branch  uuid.UUID
	anchors []uuid.UUID
}

func (f *fakeRFQPriceCatalog) ListPriceSelectionCandidates(_ context.Context, _ repository.Querier,
	account, branch uuid.UUID, anchors []uuid.UUID,
) (map[uuid.UUID][]domain.RFQPriceCandidate, error) {
	f.calls++
	f.account, f.branch, f.anchors = account, branch, anchors
	return f.groups, f.err
}

func priceSelectionProduct(name string) domain.RFQPriceCandidate {
	unit := "bolsa"
	family, subgroup := testAccountID, testBranchID
	return domain.RFQPriceCandidate{Product: domain.Product{ID: uuid.New(),
		CanonicalName: name, Unit: &unit, FamilyID: &family, SubgroupID: &subgroup, IsActive: true}}
}

func lowestPriceLine(description, search string) domain.ExtractedRFQLine {
	line := explicitLine(description, "5", "bolsa", "El cliente pidió cinco bolsas.")
	line.SearchDescription, line.SelectionPreference = search, domain.RFQSelectionLowestPrice
	return line
}

func TestRFQService_CreateTextDraft_ResolvesLowestPriceAndPreservesOriginalOrder(t *testing.T) {
	lines := []domain.ExtractedRFQLine{
		explicitLine("bolsas de cal", "10", "bolsa", "Diez bolsas."),
		explicitLine("plasticor", "2", "bolsa", "Dos bolsas."),
		lowestPriceLine("cemento mas barato que tengas", "cemento"),
	}
	h := newRFQHarness(lines)
	base, cheap := priceSelectionProduct("Cemento Loma Negra 50 kg"), priceSelectionProduct("Cemento Avellaneda 50 kg")
	h.matcher.matches[1] = domain.LineMatch{MatchStatus: domain.ItemMatchStatusNoMatch}
	h.matcher.matches[2] = domain.LineMatch{ProductID: &base.Product.ID,
		MatchStatus: domain.ItemMatchStatusAmbiguous, Confidence: decimal.RequireFromString("0.8"),
		Candidates: []domain.ScoredCandidate{scoredCandidate(base.Product.ID, base.Product.CanonicalName, "0.8")}}
	catalog := &fakeRFQPriceCatalog{groups: map[uuid.UUID][]domain.RFQPriceCandidate{base.Product.ID: {base, cheap}}}
	prices := &fakeBranchPrices{prices: map[uuid.UUID]domain.BranchPrice{
		base.Product.ID:  {Price: decimal.RequireFromString("9500"), Currency: "ARS"},
		cheap.Product.ID: {Price: decimal.RequireFromString("8900"), Currency: "ARS"},
	}}
	h.service.WithPriceSelection(NewRFQPriceSelectionService(h.db, catalog, prices, testMatchConfig()))
	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(), domain.TextRFQDraftInput{
		ChannelID: testChannelID, RawText: "Hola necesito 10 bolsas de cal, 2 de plasticor y 5 de cemento mas barato que tengas",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.matcher.descriptions, []string{"bolsas de cal", "plasticor", "cemento"}) {
		t.Fatalf("search descriptions = %v", h.matcher.descriptions)
	}
	items := h.quotes.itemBatches[0]
	if len(items) != 3 || items[1].ProductID != nil || items[1].MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Fatalf("items = %+v, want all three with Plasticor unmatched", items)
	}
	cement := items[2]
	if cement.ProductID == nil || *cement.ProductID != cheap.Product.ID || cement.MatchStatus != domain.ItemMatchStatusMatched {
		t.Fatalf("cement = %+v, want the cheapest product outside the original shortlist", cement)
	}
	if cement.RequestedDescription != lines[2].RequestedDescription || !cement.Quantity.Equal(decimal.NewFromInt(5)) ||
		cement.UnitPriceSnapshot.Valid || cement.Subtotal.Valid {
		t.Fatalf("cement = %+v, want original wording and quantity without frozen prices", cement)
	}
	if catalog.calls != 1 || prices.calls != 1 || catalog.account != testAccountID || catalog.branch != testBranchID ||
		!slices.Equal(catalog.anchors, []uuid.UUID{base.Product.ID}) || prices.branches[0] != testBranchID {
		t.Fatalf("catalog = %+v, prices = %+v, want one tenant-scoped batch each", catalog, prices)
	}
}

func TestRFQPriceSelection_Select_RespectsConstraintsAndDefersIncompleteComparisons(t *testing.T) {
	cases := []struct {
		name       string
		search     string
		preference domain.RFQSelectionPreference
		change     func(*domain.RFQPriceCandidate, map[uuid.UUID]domain.BranchPrice)
		want       string
	}{
		{name: "lowest current price", search: "cemento", want: "cheap"},
		{name: "highest current price", search: "cemento", preference: domain.RFQSelectionHighestPrice, want: "base"},
		{name: "packaging in search", search: "bolsas de cemento", want: "cheap"},
		{name: "explicit brand", search: "cemento Loma Negra", want: "base"},
		{name: "brand mentioned only in a competitor description", search: "cemento Loma Negra", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			description := "Alternative to Cemento Loma Negra 50 kg"
			p.Product.Description = &description
		}, want: "base"},
		{name: "phonetic spelling", search: "semento", want: "cheap"},
		{name: "explicit presentation", search: "cemento 50 kg", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			p.Product.CanonicalName = "Cemento Avellaneda 25 kg"
		}, want: "base"},
		{name: "unspecified mixed presentations", search: "cemento", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			p.Product.CanonicalName = "Cemento Avellaneda 25 kg"
		}},
		{name: "unknown presentation", search: "cemento", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			p.Product.CanonicalName = "Cemento Avellaneda CPN40"
		}},
		{name: "missing price", search: "cemento", change: func(p *domain.RFQPriceCandidate, prices map[uuid.UUID]domain.BranchPrice) {
			delete(prices, p.Product.ID)
		}},
		{name: "different currency", search: "cemento", change: func(p *domain.RFQPriceCandidate, prices map[uuid.UUID]domain.BranchPrice) {
			prices[p.Product.ID] = domain.BranchPrice{Price: decimal.NewFromInt(1), Currency: "USD"}
		}},
		{name: "missing classification", search: "cemento", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			p.Product.SubgroupID = nil
		}},
		{name: "different unit", search: "cemento", change: func(p *domain.RFQPriceCandidate, _ map[uuid.UUID]domain.BranchPrice) {
			unit := "kg"
			p.Product.Unit = &unit
		}},
		{name: "equal prices use stable name order", search: "cemento", change: func(p *domain.RFQPriceCandidate, prices map[uuid.UUID]domain.BranchPrice) {
			prices[p.Product.ID] = domain.BranchPrice{Price: decimal.NewFromInt(9500), Currency: "ARS"}
		}, want: "cheap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, cheap := priceSelectionProduct("Cemento Loma Negra 50 kg"), priceSelectionProduct("Cemento Avellaneda 50 kg")
			prices := map[uuid.UUID]domain.BranchPrice{
				base.Product.ID:  {Price: decimal.NewFromInt(9500), Currency: "ARS"},
				cheap.Product.ID: {Price: decimal.NewFromInt(8900), Currency: "ARS"},
			}
			if tc.change != nil {
				tc.change(&cheap, prices)
			}
			catalog := &fakeRFQPriceCatalog{groups: map[uuid.UUID][]domain.RFQPriceCandidate{base.Product.ID: {base, cheap}}}
			selector := NewRFQPriceSelectionService(&fakeRFQDB{}, catalog, &fakeBranchPrices{prices: prices}, testMatchConfig())
			matches := []domain.LineMatch{{ProductID: &base.Product.ID, MatchStatus: domain.ItemMatchStatusAmbiguous}}
			line := lowestPriceLine("cemento más barato", tc.search)
			if tc.preference != "" {
				line.SelectionPreference = tc.preference
			}
			if err := selector.Select(context.Background(), rfqTenant(), []domain.ExtractedRFQLine{line}, matches); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if matches[0].MatchStatus != domain.ItemMatchStatusAmbiguous {
					t.Fatalf("match = %+v, want seller review", matches[0])
				}
				return
			}
			want := base.Product.ID
			if tc.want == "cheap" {
				want = cheap.Product.ID
			}
			if matches[0].ProductID == nil || *matches[0].ProductID != want || matches[0].MatchStatus != domain.ItemMatchStatusMatched {
				t.Fatalf("match = %+v, want MATCHED on %s", matches[0], want)
			}
		})
	}
}

func TestRFQService_CreateTextDraft_SeparatesPriceDirectionsAndDoesNotConvertSandUnits(t *testing.T) {
	for _, unit := range []string{"mt", "m3"} {
		t.Run(unit, func(t *testing.T) {
			sand := lowestPriceLine("arena de la mas economica", "arena")
			sand.Unit = &unit
			cement := lowestPriceLine("cemento del mas caro", "cemento")
			cement.SelectionPreference = domain.RFQSelectionHighestPrice
			h := newRFQHarness([]domain.ExtractedRFQLine{sand, cement})
			fine, economy := priceSelectionProduct("Arena fina"), priceSelectionProduct("Arena eco")
			volumeUnit := "m3"
			fine.Product.Unit, economy.Product.Unit = &volumeUnit, &volumeUnit
			base, premium := priceSelectionProduct("Cemento Loma Negra 50 kg"), priceSelectionProduct("Cemento premium 50 kg")
			h.matcher.matches[0].ProductID = &fine.Product.ID
			h.matcher.matches[1].ProductID = &base.Product.ID
			catalog := &fakeRFQPriceCatalog{groups: map[uuid.UUID][]domain.RFQPriceCandidate{
				fine.Product.ID: {fine, economy}, base.Product.ID: {base, premium},
			}}
			prices := &fakeBranchPrices{prices: map[uuid.UUID]domain.BranchPrice{
				fine.Product.ID:    {Price: decimal.NewFromInt(33280), Currency: "ARS"},
				economy.Product.ID: {Price: decimal.NewFromInt(10000), Currency: "ARS"},
				base.Product.ID:    {Price: decimal.NewFromInt(9880), Currency: "ARS"},
				premium.Product.ID: {Price: decimal.NewFromInt(12480), Currency: "ARS"},
			}}
			h.service.WithPriceSelection(NewRFQPriceSelectionService(h.db, catalog, prices, testMatchConfig()))
			if _, err := h.service.CreateTextDraft(context.Background(), rfqTenant(), domain.TextRFQDraftInput{
				ChannelID: testChannelID, RawText: "necesito arena de la mas economica y cemento del mas caro",
			}); err != nil {
				t.Fatal(err)
			}
			items := h.quotes.itemBatches[0]
			if *items[1].ProductID != premium.Product.ID || items[1].MatchStatus != domain.ItemMatchStatusMatched {
				t.Fatalf("cement = %+v, want the highest current price", items[1])
			}
			if unit == "mt" && (items[0].MatchStatus != domain.ItemMatchStatusAmbiguous || *items[0].Unit != "mt") {
				t.Fatalf("sand = %+v, want review without converting metres to volume", items[0])
			}
			if unit == "m3" && (*items[0].ProductID != economy.Product.ID || items[0].MatchStatus != domain.ItemMatchStatusMatched) {
				t.Fatalf("sand = %+v, want the lowest current price in cubic metres", items[0])
			}
			if prices.calls != 1 || catalog.calls != 1 {
				t.Fatal("price directions must share a single batch")
			}
		})
	}
}

func TestRFQService_CreateTextDraft_PriceSelectionUnavailableStaysFlagged(t *testing.T) {
	for _, mode := range []string{"unwired", "catalog failure", "price failure", "no preference"} {
		t.Run(mode, func(t *testing.T) {
			line := lowestPriceLine("cemento más barato", "cemento")
			if mode == "no preference" {
				line.SelectionPreference = domain.RFQSelectionNone
			}
			h := newRFQHarness([]domain.ExtractedRFQLine{line})
			catalog, prices := &fakeRFQPriceCatalog{}, &fakeBranchPrices{}
			if mode == "catalog failure" {
				catalog.err = errors.New("catalog unavailable")
			}
			if mode == "price failure" {
				prices.err = errors.New("prices unavailable")
			}
			if mode != "unwired" {
				h.service.WithPriceSelection(NewRFQPriceSelectionService(h.db, catalog, prices, testMatchConfig()))
			}
			if _, err := h.service.CreateTextDraft(context.Background(), rfqTenant(), domain.TextRFQDraftInput{
				ChannelID: testChannelID, RawText: "5 de cemento más barato",
			}); err != nil {
				t.Fatal(err)
			}
			want := domain.ItemMatchStatusAmbiguous
			if mode == "no preference" {
				want = domain.ItemMatchStatusMatched
				if catalog.calls != 0 || prices.calls != 0 {
					t.Fatal("a line without a price preference read prices")
				}
			}
			if got := h.quotes.itemBatches[0][0].MatchStatus; got != want {
				t.Fatalf("status = %s, want %s", got, want)
			}
		})
	}
}

func TestRFQSelection_ValidatesExplicitPreferencesAndRetainsProductConstraints(t *testing.T) {
	cases := []struct {
		original, search string
		valid            bool
	}{
		{"cemento mas barato que tengas", "cemento", true},
		{"cemento más económico", "cemento", true},
		{"cemento de menor precio", "cemento", true},
		{"cemento más barato que tenés", "cemento", true},
		{"5 bolsas de cemento más barato", "cemento", true},
		{"cemento Loma Negra de 50 kg más barato", "cemento Loma Negra 50 kg", true},
		{"cemento Loma Negra más barato", "cemento", false},
		{"cemento de 50 kg más barato", "cemento", false},
		{"cemento más barato", "cemento Avellaneda", false},
		{"cemento", "cemento", true},
		{"cemento al precio más bajo", "cemento", true},
		{"cemento más barato que consigas", "cemento", true},
		{"semento más barato", "cemento", true},
		{"cemento más barato", "", false},
		{"cemento más barato", strings.Repeat("c", 513), false},
	}
	for _, tc := range cases {
		t.Run(tc.original+"/"+tc.search, func(t *testing.T) {
			_, err := newQuoteItemsFromRFQLines([]domain.ExtractedRFQLine{lowestPriceLine(tc.original, tc.search)})
			if (err == nil) != tc.valid {
				t.Fatalf("validation error = %v, want valid = %v", err, tc.valid)
			}
		})
	}
}

func TestRFQSelection_ValidatesHighestPriceSynonyms(t *testing.T) {
	line := lowestPriceLine("cemento más costoso", "cemento")
	line.SelectionPreference = domain.RFQSelectionHighestPrice
	if _, err := newQuoteItemsFromRFQLines([]domain.ExtractedRFQLine{line}); err != nil {
		t.Fatal(err)
	}
}

func TestRFQService_CreateTextDraft_PreservesOrdinaryMatchingDescriptions(t *testing.T) {
	line := explicitLine("cemento Loma Negra de 50 kg", "5", "bolsa", "Five bags.")
	line.SearchDescription = "cemento"
	line.SelectionPreference = domain.RFQSelectionNone
	h := newRFQHarness([]domain.ExtractedRFQLine{line})
	if _, err := h.service.CreateTextDraft(context.Background(), rfqTenant(), domain.TextRFQDraftInput{
		ChannelID: testChannelID, RawText: "5 bolsas de cemento Loma Negra de 50 kg",
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.matcher.descriptions, []string{line.RequestedDescription}) {
		t.Fatalf("search descriptions = %v, want original product constraints", h.matcher.descriptions)
	}
}
