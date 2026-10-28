package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// fakePromotions serves fixed rules and taxonomy to a sweep.
type fakePromotions struct {
	rules  []domain.Promotion
	groups map[uuid.UUID]domain.ProductGroup
	at     time.Time
}

func (f *fakePromotions) ListInForce(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, at time.Time,
) ([]domain.Promotion, error) {
	f.at = at
	return f.rules, nil
}

func (f *fakePromotions) ListProductGroups(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ []uuid.UUID,
) (map[uuid.UUID]domain.ProductGroup, error) {
	return f.groups, nil
}

type sweepFixture struct {
	sweep     discountSweep
	discounts *fakeQuoteDiscounts
	account   uuid.UUID
	version   uuid.UUID
	items     []domain.QuoteItem
}

// newSweepFixture prices two lines: A, 10 × 1000.00 = 10000.00; B, 5 × 1000.00 = 5000.00.
func newSweepFixture(rules ...domain.Promotion) *sweepFixture {
	f := &sweepFixture{discounts: &fakeQuoteDiscounts{links: map[uuid.UUID][]uuid.UUID{}},
		account: uuid.New(), version: uuid.New()}
	f.sweep = discountSweep{discounts: f.discounts, promotions: &fakePromotions{rules: rules},
		now: func() time.Time { return engineAt }}
	for _, line := range []struct {
		id, quantity, subtotal string
		product                uuid.UUID
	}{{lineA, "10", "10000.00", cement}, {lineB, "5", "5000.00", sand}} {
		product := line.product
		f.items = append(f.items, domain.QuoteItem{ID: uuid.MustParse(line.id), ProductID: &product,
			Quantity: dec(line.quantity), UnitPriceSnapshot: decimal.NewNullDecimal(dec("1000.00")),
			Subtotal: decimal.NewNullDecimal(dec(line.subtotal))})
	}
	return f
}

func (f *sweepFixture) settle(t *testing.T) decimal.Decimal {
	t.Helper()
	total, err := f.sweep.settle(context.Background(), nil, f.account, engineBranch, f.version, f.items)
	if err != nil {
		t.Fatalf("settle() = %v", err)
	}
	return total
}

func (f *sweepFixture) automatic() []domain.QuoteDiscount {
	var out []domain.QuoteDiscount
	for _, d := range f.discounts.stored {
		if d.Origin == domain.DiscountOriginAutomatic {
			out = append(out, d)
		}
	}
	return out
}

func tenPercentOnCement() domain.Promotion {
	return promo{id: promoP, condition: domain.PromotionConditionPerItem,
		action: domain.PromotionActionPercentage, value: "10", targets: []uuid.UUID{cement}}.build()
}

func TestDiscountSweep_WritesAndKeepsItsApplications(t *testing.T) {
	t.Parallel()
	f := newSweepFixture(tenPercentOnCement())

	// 15000 − 10000 × 10% = 14000.
	if total := f.settle(t); !total.Equal(dec("14000.00")) {
		t.Fatalf("total = %s, want 14000.00", total)
	}
	first := f.automatic()
	if len(first) != 1 || !first[0].Amount.Equal(dec("1000.00")) {
		t.Fatalf("automatic = %+v, want one 1000.00 application", first)
	}

	// The same lines again: the same row, untouched, and the same total.
	if total := f.settle(t); !total.Equal(dec("14000.00")) {
		t.Fatalf("second total = %s, want 14000.00", total)
	}
	again := f.automatic()
	if len(again) != 1 || again[0].ID != first[0].ID {
		t.Fatalf("second sweep rows = %+v, want the first row kept", again)
	}

	// The line grows to 20 bags (20000.00): the row follows it in place. 25000 − 2000 = 23000.
	f.items[0].Quantity, f.items[0].Subtotal = dec("20"), decimal.NewNullDecimal(dec("20000.00"))
	if total := f.settle(t); !total.Equal(dec("23000.00")) {
		t.Fatalf("total after the change = %s, want 23000.00", total)
	}
	moved := f.automatic()
	if len(moved) != 1 || moved[0].ID != first[0].ID || !moved[0].Amount.Equal(dec("2000.00")) {
		t.Fatalf("rows after the change = %+v, want the same row at 2000.00", moved)
	}

	// The line goes: the application goes with it. 5000 − 0.
	f.items = f.items[1:]
	if total := f.settle(t); !total.Equal(dec("5000.00")) {
		t.Fatalf("total without the line = %s, want 5000.00", total)
	}
	if rows := f.automatic(); len(rows) != 0 {
		t.Fatalf("rows without the line = %+v, want none", rows)
	}
}

func TestDiscountSweep_LeavesSuppressedAndSellerDiscountsAlone(t *testing.T) {
	t.Parallel()
	f := newSweepFixture(tenPercentOnCement())
	f.settle(t)
	f.discounts.stored[0].SuppressedBySeller = true
	suppressedID := f.discounts.stored[0].ID

	manualValue := dec("500.00")
	manual := domain.QuoteDiscount{ID: uuid.New(), AccountID: f.account, QuoteVersionID: f.version,
		Scope: domain.DiscountScopeTotal, Origin: domain.DiscountOriginManualSeller,
		Amount: dec("500.00"), ActionType: domain.PromotionActionFixedAmount, ActionValue: &manualValue}
	f.discounts.stored = append(f.discounts.stored, manual)

	// The suppressed row neither counts nor comes back: 15000 − 500 (the seller's) = 14500.
	if total := f.settle(t); !total.Equal(dec("14500.00")) {
		t.Fatalf("total = %s, want 14500.00", total)
	}
	if len(f.discounts.stored) != 2 || f.discounts.stored[0].ID != suppressedID ||
		!f.discounts.stored[0].SuppressedBySeller {
		t.Fatalf("stored = %+v, want the suppressed row kept and nothing re-applied", f.discounts.stored)
	}
}

func TestDiscountSweep_TotalNeverNegative(t *testing.T) {
	t.Parallel()
	f := newSweepFixture()
	value := dec("20000.00")
	f.discounts.stored = append(f.discounts.stored, domain.QuoteDiscount{ID: uuid.New(),
		AccountID: f.account, QuoteVersionID: f.version, Scope: domain.DiscountScopeTotal,
		Origin: domain.DiscountOriginManualSeller, Amount: value,
		ActionType: domain.PromotionActionFixedAmount, ActionValue: &value})
	if total := f.settle(t); !total.Equal(decimal.Zero) {
		t.Fatalf("total = %s, want 0", total)
	}
}

func TestDiscountSweep_ReadsTheRulesAtItsOwnClock(t *testing.T) {
	t.Parallel()
	f := newSweepFixture(tenPercentOnCement())
	f.settle(t)
	if at := f.sweep.promotions.(*fakePromotions).at; !at.Equal(engineAt) {
		t.Fatalf("rules read at %s, want %s", at, engineAt)
	}
}

func TestDiscountSweep_UsesTheRulesFrozenWithAClientSelection(t *testing.T) {
	t.Parallel()
	f := newSweepFixture()
	snapshot := domain.QuoteSelectionPricingSnapshot{CapturedAt: engineAt,
		Promotions: []domain.Promotion{tenPercentOnCement()}}

	total, err := f.sweep.settleWithSnapshot(context.Background(), nil, f.account, engineBranch,
		f.version, f.items, snapshot)
	if err != nil {
		t.Fatalf("settleWithSnapshot() = %v", err)
	}
	if !total.Equal(dec("14000.00")) {
		t.Fatalf("total = %s, want 14000.00 from the sent promotion", total)
	}
	if applications := f.automatic(); len(applications) != 1 || !applications[0].Amount.Equal(dec("1000.00")) {
		t.Fatalf("applications = %+v, want the sent 1000.00 promotion", applications)
	}
}
