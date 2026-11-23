package services

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var pricePreferenceWording = map[domain.RFQSelectionPreference]*regexp.Regexp{
	domain.RFQSelectionLowestPrice:  regexp.MustCompile(`\b(?:mas\s+(?:barat[oa]s?|economic[oa]s?)|menor\s+precio|precio\s+mas\s+bajo)(?:\s+(?:que\s+(?:tengas?|tenes|tienen|haya|consigas?|conseguis)|disponible|posible))?\b`),
	domain.RFQSelectionHighestPrice: regexp.MustCompile(`\b(?:mas\s+(?:car[oa]s?|costos[oa]s?)|mayor\s+precio|precio\s+mas\s+alto)(?:\s+(?:que\s+(?:tengas?|tenes|tienen|haya|consigas?|conseguis)|disponible|posible))?\b`),
}

type rfqPriceCatalog interface {
	ListPriceSelectionCandidates(context.Context, repository.Querier, uuid.UUID, uuid.UUID,
		[]uuid.UUID) (map[uuid.UUID][]domain.RFQPriceCandidate, error)
}

type rfqPriceSelector interface {
	Select(context.Context, domain.Tenant, []domain.ExtractedRFQLine, []domain.LineMatch) error
}

// RFQPriceSelectionService resolves explicit price preferences before draft valorization.
type RFQPriceSelectionService struct {
	db      tenantTxRunner
	catalog rfqPriceCatalog
	prices  branchPriceReader
	scorer  *CatalogMatchService
}

// NewRFQPriceSelectionService builds a selector using the current branch catalog and prices.
func NewRFQPriceSelectionService(db tenantTxRunner, catalog rfqPriceCatalog,
	prices branchPriceReader, cfg config.CatalogConfig,
) *RFQPriceSelectionService {
	return &RFQPriceSelectionService{db: db, catalog: catalog, prices: prices,
		scorer: NewCatalogMatchService(nil, cfg)}
}

// Select compares all compatible products in the matched subgroup without freezing their prices.
func (s *RFQPriceSelectionService) Select(ctx context.Context, tenant domain.Tenant,
	lines []domain.ExtractedRFQLine, matches []domain.LineMatch,
) error {
	if !tenant.HasBranch() || len(lines) != len(matches) {
		return fmt.Errorf("%w: price selection needs a branch and aligned lines", domain.ErrInvalidInput)
	}
	var anchors []uuid.UUID
	for i, line := range lines {
		if pricePreferenceWording[line.SelectionPreference] != nil && matches[i].ProductID != nil {
			anchors = append(anchors, *matches[i].ProductID)
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	var groups map[uuid.UUID][]domain.RFQPriceCandidate
	var prices map[uuid.UUID]domain.BranchPrice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		groups, err = s.catalog.ListPriceSelectionCandidates(ctx, q, tenant.AccountID, tenant.BranchID, anchors)
		if err != nil {
			return err
		}
		ids := make(map[uuid.UUID]bool)
		for _, group := range groups {
			for _, candidate := range group {
				ids[candidate.Product.ID] = true
			}
		}
		productIDs := make([]uuid.UUID, 0, len(ids))
		for id := range ids {
			productIDs = append(productIDs, id)
		}
		prices, err = s.prices.GetCurrentByProductIDs(ctx, q, tenant.AccountID, tenant.BranchID, productIDs)
		return err
	})
	if err != nil {
		return err
	}
	for i, line := range lines {
		if pricePreferenceWording[line.SelectionPreference] == nil || matches[i].ProductID == nil {
			continue
		}
		matches[i] = s.selectByPrice(line, matches[i], groups[*matches[i].ProductID], prices)
	}
	return nil
}

func (s *RFQPriceSelectionService) selectByPrice(line domain.ExtractedRFQLine,
	match domain.LineMatch, group []domain.RFQPriceCandidate, prices map[uuid.UUID]domain.BranchPrice,
) domain.LineMatch {
	query := slices.DeleteFunc(lineTokens(line.SearchDescription), func(t matchToken) bool { return t.packaging })
	var candidates []domain.ScoredCandidate
	var base *domain.Product
	var presentation string
	var currency string
	for _, candidate := range group {
		p := candidate.Product
		catalog := domain.CatalogCandidate{ProductID: p.ID, Code: p.Code,
			CanonicalName: p.CanonicalName, Description: p.Description, Unit: p.Unit, Synonyms: candidate.Synonyms}
		identity := catalog
		identity.Description = nil
		// Free descriptions may mention competing brands without identifying this product.
		if !priceConstraintsCovered(query, candidateTokens(identity)) {
			continue
		}
		if !p.IsActive || !compatibleRecommendation(p, p) {
			return match
		}
		if line.Unit != nil && priceUnitKey(*line.Unit) != priceUnitKey(*p.Unit) {
			return match
		}
		spec := pricePresentation(p)
		if spec == "" || (base != nil && (!compatibleRecommendation(*base, p) || spec != presentation)) {
			return match
		}
		if base == nil {
			base, presentation = &p, spec
		}
		price, ok := prices[p.ID]
		if !ok || price.Currency == "" || (currency != "" && currency != price.Currency) {
			return match
		}
		currency = price.Currency
		confidence, _, _ := s.scorer.confidenceOf(query, catalog)
		if confidence.LessThan(s.scorer.minConfidence) {
			return match
		}
		candidates = append(candidates, domain.ScoredCandidate{CatalogCandidate: catalog, Confidence: confidence})
	}
	if len(candidates) == 0 {
		return match
	}
	slices.SortFunc(candidates, func(a, b domain.ScoredCandidate) int {
		if order := prices[a.ProductID].Price.Cmp(prices[b.ProductID].Price); order != 0 {
			if line.SelectionPreference == domain.RFQSelectionHighestPrice {
				return -order
			}
			return order
		}
		if order := strings.Compare(a.CanonicalName, b.CanonicalName); order != 0 {
			return order
		}
		return strings.Compare(a.ProductID.String(), b.ProductID.String())
	})
	leader := candidates[0]
	match.ProductID, match.Confidence = &leader.ProductID, leader.Confidence
	match.Candidates, match.SettledByReview = candidates, false
	match.MatchStatus = domain.ItemMatchStatusMatched
	return match
}

func pricePresentation(product domain.Product) string {
	var figures []string
	measured := false
	for _, token := range tokenizeCatalogText(product.CanonicalName) {
		if token.kind == tokenFigure {
			figures = append(figures, token.text+":"+token.unit)
			measured = measured || token.unit != ""
		}
	}
	if measured {
		return strings.Join(figures, "|")
	}
	// Counted packages need an explicit presentation; base measurement units compare directly.
	unit := strings.ToLower(strings.TrimSpace(*product.Unit))
	switch unit {
	case "kg", "m", "m2", "m3", "l":
		return unit
	}
	return ""
}

func priceUnitKey(unit string) string {
	unit = strings.Trim(strings.ToLower(strings.TrimSpace(unit)), ".")
	switch unit {
	case "bol", "bolsa", "bolsas":
		return "bolsa"
	case "u", "un", "unidad", "unidades":
		return "unidad"
	}
	if normalized, ok := unitWords[unit]; ok {
		return normalized
	}
	return unit
}

func priceConstraintsCovered(query, document []matchToken) bool {
	if len(query) == 0 {
		return false
	}
	spent := make([]bool, len(document))
	for _, token := range query {
		best, bestAt := 0.0, -1
		for i, candidate := range document {
			if !spent[i] {
				if credit := tokenCredit(token, candidate); credit > best {
					best, bestAt = credit, i
				}
			}
		}
		if best < editDistanceCredit || (token.kind != tokenWord && best != 1) {
			return false
		}
		spent[bestAt] = true
	}
	return true
}

func validateRFQSelection(line domain.ExtractedRFQLine, field string) error {
	if err := requireMaxRunes(line.SearchDescription, field+".search_description", 512); err != nil {
		return err
	}
	switch line.SelectionPreference {
	case "", domain.RFQSelectionNone:
		return nil
	case domain.RFQSelectionLowestPrice, domain.RFQSelectionHighestPrice:
		wording := pricePreferenceWording[line.SelectionPreference]
		search := strings.TrimSpace(line.SearchDescription)
		if search == "" {
			return fmt.Errorf("%w: %s.search_description is required for price selection", domain.ErrInvalidInput, field)
		}
		original := accentFolder.Replace(strings.ToLower(line.RequestedDescription))
		// Shared preferences can be outside the item's verbatim description.
		identity := slices.DeleteFunc(lineTokens(wording.ReplaceAllString(original, " ")),
			func(t matchToken) bool { return t.packaging })
		query := slices.DeleteFunc(lineTokens(search), func(t matchToken) bool { return t.packaging })
		if !priceConstraintsCovered(identity, query) || !priceConstraintsCovered(query, identity) {
			return fmt.Errorf("%w: %s.search_description must preserve explicit product constraints", domain.ErrInvalidInput, field)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s.selection_preference is unknown", domain.ErrInvalidInput, field)
	}
}
