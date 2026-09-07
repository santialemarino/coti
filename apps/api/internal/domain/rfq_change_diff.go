package domain

import "github.com/shopspring/decimal"

// ChangeRequestSnapshot is one side of the change-request comparison: the frozen
// version the client saw, or the draft the seller is rebuilding. Both come from the
// same persistence reads the detail screen uses, so nothing here re-derives them.
type ChangeRequestSnapshot struct {
	Items     []QuoteItem
	Discounts []QuoteDiscount
	Total     decimal.Decimal
}

// ChangeRequestDiff compares the frozen version a client saw against the draft the
// seller is rebuilding. Populated only when a CHANGE_REQUESTED quote has a frozen
// predecessor. Original is the client-facing snapshot; Requested carries the flags
// the diff screen paints.
type ChangeRequestDiff struct {
	Reason    *string
	Original  ChangeRequestSide
	Requested ChangeRequestSide
}

// ChangeRequestSide is one column (original or requested) of the rendered diff.
type ChangeRequestSide struct {
	Items     []DiffLineItem
	Discounts []DiffDiscountLine
	Total     decimal.Decimal
}

// DiffLineItem is one aligned line of the comparison. Quantity and UnitPrice are
// decimal strings; ChangeType separates added, removed and modified lines.
type DiffLineItem struct {
	Description string
	Quantity    string
	Unit        *string
	UnitPrice   *string
	Changed     bool
	ChangeType  string
}

// DiffDiscountLine is one aligned discount row of the comparison.
type DiffDiscountLine struct {
	Name    string
	Amount  string
	Changed bool
}

// BuildChangeRequestDiff aligns the frozen and draft item lists by line position —
// the same walk the diff screen performs — and flags lines and discounts that
// differ. Lines past the end of the frozen list are added; frozen lines past the end
// of the draft list are emitted as removed placeholders.
func BuildChangeRequestDiff(reason *string, frozen, draft ChangeRequestSnapshot) ChangeRequestDiff {
	return ChangeRequestDiff{
		Reason:    reason,
		Original:  side(frozen),
		Requested: requestedSide(draft, frozen),
	}
}

func side(snapshot ChangeRequestSnapshot) ChangeRequestSide {
	side := ChangeRequestSide{
		Items:     make([]DiffLineItem, 0, len(snapshot.Items)),
		Discounts: make([]DiffDiscountLine, 0, len(snapshot.Discounts)),
		Total:     snapshot.Total,
	}
	for _, item := range snapshot.Items {
		side.Items = append(side.Items, DiffLineItem{
			Description: item.RequestedDescription,
			Quantity:    item.Quantity.StringFixed(MoneyScale),
			Unit:        item.Unit,
			UnitPrice:   priceString(item.UnitPriceSnapshot),
		})
	}
	for _, discount := range snapshot.Discounts {
		side.Discounts = append(side.Discounts, DiffDiscountLine{
			Name:   discountName(discount),
			Amount: discount.Amount.StringFixed(MoneyScale),
		})
	}
	return side
}

// requestedSide paints the draft's changes against the frozen list: modified for
// position-aligned lines that differ, added past the end of the frozen list, removed
// for the frozen lines the draft dropped. Discounts are matched by name; a changed
// amount or suppression flags the draft row.
func requestedSide(draft, frozen ChangeRequestSnapshot) ChangeRequestSide {
	side := ChangeRequestSide{
		Items:     make([]DiffLineItem, 0, len(draft.Items)),
		Discounts: make([]DiffDiscountLine, 0, len(draft.Discounts)),
		Total:     draft.Total,
	}
	for i, item := range draft.Items {
		changed, changeType := true, "added"
		if i < len(frozen.Items) {
			if sameItem(frozen.Items[i], item) {
				changed, changeType = false, ""
			} else {
				changeType = "modified"
			}
		}
		side.Items = append(side.Items, DiffLineItem{
			Description: item.RequestedDescription,
			Quantity:    item.Quantity.StringFixed(MoneyScale),
			Unit:        item.Unit,
			UnitPrice:   priceString(item.UnitPriceSnapshot),
			Changed:     changed,
			ChangeType:  changeType,
		})
	}
	if len(draft.Items) < len(frozen.Items) {
		for _, removed := range frozen.Items[len(draft.Items):] {
			side.Items = append(side.Items, DiffLineItem{
				Description: removed.RequestedDescription,
				Changed:     true,
				ChangeType:  "removed",
			})
		}
	}

	byName := make(map[string]QuoteDiscount, len(frozen.Discounts))
	for _, discount := range frozen.Discounts {
		byName[discountName(discount)] = discount
	}
	for _, discount := range draft.Discounts {
		name := discountName(discount)
		previous, matched := byName[name]
		changed := !matched ||
			!discount.Amount.Equal(previous.Amount) ||
			discount.SuppressedBySeller != previous.SuppressedBySeller
		side.Discounts = append(side.Discounts, DiffDiscountLine{
			Name:    name,
			Amount:  discount.Amount.StringFixed(MoneyScale),
			Changed: changed,
		})
	}
	return side
}

func sameItem(frozen, draft QuoteItem) bool {
	return frozen.RequestedDescription == draft.RequestedDescription &&
		frozen.Quantity.Equal(draft.Quantity) &&
		equalStringPtr(frozen.Unit, draft.Unit) &&
		equalPrice(frozen.UnitPriceSnapshot, draft.UnitPriceSnapshot)
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
