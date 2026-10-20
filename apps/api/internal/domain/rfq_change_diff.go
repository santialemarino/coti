package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ChangeRequestSnapshot is one side of the change-request comparison: the frozen
// version the client saw, or the draft the seller is rebuilding. Both come from the
// same persistence reads the detail screen uses, so nothing here re-derives them.
type ChangeRequestSnapshot struct {
	Items     []QuoteItem
	Discounts []QuoteDiscount
	Total     decimal.Decimal
}

// ChangeRequestDiff compares the frozen version a client saw against the draft the seller is
// rebuilding. Populated only when a CHANGE_REQUESTED quote has a frozen predecessor.
type ChangeRequestDiff struct {
	Reason *string
	// Lines pairs each sent line with the draft line copied from it, then the lines the draft added.
	Lines          []DiffLine
	Discounts      []DiffDiscount
	OriginalTotal  decimal.Decimal
	RequestedTotal decimal.Decimal
	// Handler is the latest conversation handler decision on the draft, when one ran.
	Handler *HandlerDecisionNote
	// ClientMessages is every message the customer sent since the version they answered went out.
	ClientMessages []QuoteMessage
}

// DiffChange says how a draft row differs from what the customer received.
type DiffChange string

const (
	DiffUnchanged DiffChange = "UNCHANGED"
	DiffModified  DiffChange = "MODIFIED"
	DiffAdded     DiffChange = "ADDED"
	DiffRemoved   DiffChange = "REMOVED"
)

// DiffLine is one line of the comparison: the sent line, the draft line, or both.
type DiffLine struct {
	Change    DiffChange
	Original  *DiffLineItem
	Requested *DiffLineItem
}

// DiffLineItem is one side of a compared line. Quantity and UnitPrice are decimal strings.
type DiffLineItem struct {
	Description string
	ProductName *string
	Quantity    string
	Unit        *string
	UnitPrice   *string
}

// DiffDiscount is one discount of the comparison, paired by promotion when it has one, else by name.
type DiffDiscount struct {
	Change    DiffChange
	Name      string
	Original  *DiffDiscountSide
	Requested *DiffDiscountSide
}

// DiffDiscountSide is a discount's money and suppression on one side of the comparison.
type DiffDiscountSide struct {
	Amount     string
	Suppressed bool
	// Origin is shown, not compared: a promotion the AI reached is the same promotion.
	Origin DiscountOrigin
}

// BuildChangeRequestDiff pairs each frozen line with the draft line copied from it, by the draft
// line's source, so a reordered or swapped line is compared with its own predecessor. Frozen lines
// no draft line came from are removed; draft lines with no frozen source are added.
func BuildChangeRequestDiff(reason *string, frozen, draft ChangeRequestSnapshot) ChangeRequestDiff {
	diff := ChangeRequestDiff{Reason: reason, Lines: make([]DiffLine, 0, len(draft.Items)),
		Discounts: make([]DiffDiscount, 0), OriginalTotal: frozen.Total, RequestedTotal: draft.Total}

	used := make(map[int]bool, len(draft.Items))
	for _, sent := range frozen.Items {
		paired := -1
		for i, line := range draft.Items {
			if !used[i] && line.SourceItemID != nil && *line.SourceItemID == sent.ID {
				paired = i
				break
			}
		}
		original := diffLineItem(sent)
		if paired < 0 {
			diff.Lines = append(diff.Lines, DiffLine{Change: DiffRemoved, Original: &original})
			continue
		}
		used[paired] = true
		requested := diffLineItem(draft.Items[paired])
		change := DiffModified
		if sameItem(sent, draft.Items[paired]) {
			change = DiffUnchanged
		}
		diff.Lines = append(diff.Lines, DiffLine{Change: change, Original: &original,
			Requested: &requested})
	}
	for i, line := range draft.Items {
		if !used[i] {
			requested := diffLineItem(line)
			diff.Lines = append(diff.Lines, DiffLine{Change: DiffAdded, Requested: &requested})
		}
	}

	sources := make(map[uuid.UUID]uuid.UUID, len(draft.Items))
	for _, line := range draft.Items {
		if line.SourceItemID != nil {
			sources[line.ID] = *line.SourceItemID
		}
	}
	sent := make(map[string]QuoteDiscount, len(frozen.Discounts))
	for _, discount := range frozen.Discounts {
		sent[discountKey(discount, nil)] = discount
	}
	seen := make(map[string]bool, len(draft.Discounts))
	for _, discount := range draft.Discounts {
		name, key := discountName(discount), discountKey(discount, sources)
		seen[key] = true
		requested := discountSide(discount)
		previous, ok := sent[key]
		if !ok {
			diff.Discounts = append(diff.Discounts, DiffDiscount{Change: DiffAdded, Name: name,
				Requested: &requested})
			continue
		}
		original := discountSide(previous)
		change := DiffUnchanged
		if original.Amount != requested.Amount || original.Suppressed != requested.Suppressed {
			change = DiffModified
		}
		diff.Discounts = append(diff.Discounts, DiffDiscount{Change: change, Name: name,
			Original: &original, Requested: &requested})
	}
	for _, discount := range frozen.Discounts {
		if !seen[discountKey(discount, nil)] {
			original := discountSide(discount)
			diff.Discounts = append(diff.Discounts, DiffDiscount{Change: DiffRemoved,
				Name: discountName(discount), Original: &original})
		}
	}
	return diff
}

// discountKey pairs a draft discount with the sent one it continues: a promotion's by the promotion
// and, for one line, the sent line that draft line was copied from (sources maps draft lines to
// sent ones; nil reads the sent side); anything else by its name.
func discountKey(discount QuoteDiscount, sources map[uuid.UUID]uuid.UUID) string {
	if discount.PromotionID == nil {
		return "name:" + discountName(discount)
	}
	key := "promotion:" + discount.PromotionID.String()
	if discount.Scope != DiscountScopeItem || len(discount.ItemIDs) != 1 {
		return key
	}
	line := discount.ItemIDs[0]
	if sources != nil {
		source, ok := sources[line]
		if !ok {
			return key + "/added:" + line.String()
		}
		line = source
	}
	return key + "/" + line.String()
}

func diffLineItem(item QuoteItem) DiffLineItem {
	return DiffLineItem{Description: item.RequestedDescription, ProductName: item.ProductName,
		Quantity: item.Quantity.StringFixed(MoneyScale), Unit: item.Unit,
		UnitPrice: priceString(item.UnitPriceSnapshot)}
}

func discountSide(discount QuoteDiscount) DiffDiscountSide {
	return DiffDiscountSide{Amount: discount.Amount.StringFixed(MoneyScale),
		Suppressed: discount.SuppressedBySeller, Origin: discount.Origin}
}

func sameItem(frozen, draft QuoteItem) bool {
	return frozen.RequestedDescription == draft.RequestedDescription &&
		equalUUIDPtr(frozen.ProductID, draft.ProductID) &&
		frozen.Quantity.Equal(draft.Quantity) &&
		equalStringPtr(frozen.Unit, draft.Unit) &&
		equalPrice(frozen.UnitPriceSnapshot, draft.UnitPriceSnapshot)
}

func equalUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalPrice(a, b decimal.NullDecimal) bool {
	if !a.Valid || !b.Valid {
		return a.Valid == b.Valid
	}
	return a.Decimal.Equal(b.Decimal)
}

func priceString(price decimal.NullDecimal) *string {
	if !price.Valid {
		return nil
	}
	return strPtr(price.Decimal.StringFixed(MoneyScale))
}

func discountName(discount QuoteDiscount) string {
	if discount.PromotionName != nil {
		return *discount.PromotionName
	}
	if discount.Description != nil {
		return *discount.Description
	}
	return ""
}

func strPtr(s string) *string {
	return &s
}
