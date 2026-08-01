package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The price-validity rule is the only real logic in the catalog ticket, so it is tested
// here against fakes: that setting a price closes the open period at the instant the new
// one starts, and that nothing is ever overwritten. The SQL side — one open period left in
// the table, the history intact — is asserted against a real database in
// internal/repository.

var testBranchID = uuid.MustParse("55555555-5555-4555-8555-555555555555")

// closedPeriod records a call to CloseOpenPeriod so a test can assert when the previous
// price stopped applying.
type closedPeriod struct {
	productID uuid.UUID
	at        time.Time
}

type fakePrices struct {
	open    *domain.ProductPrice
	closed  []closedPeriod
	created []domain.NewProductPrice
	branch  []uuid.UUID
}

func (f *fakePrices) ListByProduct(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, branchID *uuid.UUID,
) ([]domain.ProductPrice, error) {
	if branchID != nil {
		f.branch = append(f.branch, *branchID)
	}
	return nil, nil
}

func (f *fakePrices) GetOpenPeriod(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
) (*domain.ProductPrice, error) {
	if f.open == nil {
		return nil, domain.ErrNotFound
	}
	return f.open, nil
}

func (f *fakePrices) Create(
	_ context.Context, _ repository.Querier, accountID, branchID, productID uuid.UUID,
	userID *uuid.UUID, in domain.NewProductPrice,
) (*domain.ProductPrice, error) {
	f.created = append(f.created, in)
	return &domain.ProductPrice{
		ID: uuid.New(), AccountID: accountID, BranchID: branchID, ProductID: productID,
		UserID: userID, Price: in.Price, Currency: in.Currency, MinPrice: in.MinPrice,
		ValidFrom: in.ValidFrom,
	}, nil
}

func (f *fakePrices) CloseOpenPeriod(
	_ context.Context, _ repository.Querier, _, _, productID uuid.UUID, at time.Time,
) (int64, error) {
	f.closed = append(f.closed, closedPeriod{productID: productID, at: at})
	return 1, nil
}

type fakeAvailability struct {
	saved  []domain.BranchAvailability
	branch []uuid.UUID
}

func (f *fakeAvailability) ListByProduct(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, branchID *uuid.UUID,
) ([]domain.BranchProduct, error) {
	if branchID != nil {
		f.branch = append(f.branch, *branchID)
	}
	return nil, nil
}

func (f *fakeAvailability) Save(
	_ context.Context, _ repository.Querier, accountID, branchID, productID uuid.UUID,
	in domain.BranchAvailability,
) (*domain.BranchProduct, error) {
	f.saved = append(f.saved, in)
	return &domain.BranchProduct{
		ID: uuid.New(), AccountID: accountID, BranchID: branchID, ProductID: productID,
		Stock: in.Stock, IsActive: in.IsActive,
	}, nil
}

type pricingHarness struct {
	service      *BranchCatalogService
	db           *fakeDB
	products     *fakeProducts
	availability *fakeAvailability
	prices       *fakePrices
}

func newPricingHarness(open *domain.ProductPrice, known ...uuid.UUID) *pricingHarness {
	h := &pricingHarness{
		db:           &fakeDB{},
		products:     newFakeProducts(known...),
		availability: &fakeAvailability{},
		prices:       &fakePrices{open: open},
	}
	h.service = NewBranchCatalogService(h.db, h.products, h.availability, h.prices,
		func() time.Time { return fixedNow })
	return h
}

func branchTenant() domain.Tenant {
	return domain.Tenant{
		AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin,
		BranchID: testBranchID,
	}
}

func amount(s string) decimal.NullDecimal {
	return decimal.NullDecimal{Decimal: decimal.RequireFromString(s), Valid: true}
}

// The core of the ticket: an update opens a period and closes the previous one at the same
// instant, instead of rewriting the row.
func TestBranchCatalogService_SetPrice_ClosesThePreviousPeriodAtTheNewStart(t *testing.T) {
	previousStart := fixedNow.Add(-72 * time.Hour)
	newStart := fixedNow.Add(24 * time.Hour)
	h := newPricingHarness(&domain.ProductPrice{
		ProductID: testProductID,
		Price:     decimal.RequireFromString("8500.00"),
		ValidFrom: previousStart,
	}, testProductID)

	created, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID,
		domain.NewProductPrice{
			Price:     decimal.RequireFromString("9100.00"),
			MinPrice:  amount("8600.00"),
			ValidFrom: newStart,
		})
	if err != nil {
		t.Fatalf("SetPrice() = %v, want no error", err)
	}

	if len(h.prices.closed) != 1 {
		t.Fatalf("periods closed = %d, want 1", len(h.prices.closed))
	}
	if got := h.prices.closed[0].at; !got.Equal(newStart) {
		t.Errorf("previous period closed at %v, want %v — the periods must meet exactly", got, newStart)
	}
	if got := h.prices.closed[0].productID; got != testProductID {
		t.Errorf("closed the period of product %v, want %v", got, testProductID)
	}
	if len(h.prices.created) != 1 {
		t.Fatalf("periods opened = %d, want 1", len(h.prices.created))
	}
	if got := h.prices.created[0].ValidFrom; !got.Equal(newStart) {
		t.Errorf("new period starts at %v, want %v", got, newStart)
	}
	if created.UserID == nil || *created.UserID != testUserID {
		t.Errorf("set_by = %v, want the caller %v", created.UserID, testUserID)
	}
	// One transaction for the whole thing: a crash between the close and the insert cannot
	// leave the product with two open periods or none.
	if len(h.db.scopes) != 1 {
		t.Errorf("tenant-scoped transactions opened = %d, want 1", len(h.db.scopes))
	}
}

// The product row is locked, not merely read: two repricings racing on the same product
// would otherwise each read the same open period and leave two of them open.
func TestBranchCatalogService_SetPrice_LocksTheProduct(t *testing.T) {
	h := newPricingHarness(nil, testProductID)

	if _, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID,
		domain.NewProductPrice{Price: decimal.RequireFromString("8500.00")}); err != nil {
		t.Fatalf("SetPrice() = %v, want no error", err)
	}
	if len(h.products.locked) != 1 || h.products.locked[0] != testProductID {
		t.Errorf("locking reads = %v, want [%v]", h.products.locked, testProductID)
	}

	// A read has no reason to hold the row, and holding it would serialize listings.
	reads := newPricingHarness(nil, testProductID)
	if _, err := reads.service.ListPrices(context.Background(), branchTenant(), testProductID); err != nil {
		t.Fatalf("ListPrices() = %v, want no error", err)
	}
	if len(reads.products.locked) != 0 {
		t.Errorf("locking reads during a listing = %v, want none", reads.products.locked)
	}
}

func TestBranchCatalogService_SetPrice_FirstPriceClosesNothing(t *testing.T) {
	h := newPricingHarness(nil, testProductID)

	if _, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID,
		domain.NewProductPrice{Price: decimal.RequireFromString("8500.00")}); err != nil {
		t.Fatalf("SetPrice() = %v, want no error", err)
	}

	if len(h.prices.closed) != 0 {
		t.Errorf("periods closed = %d, want 0 for the first price", len(h.prices.closed))
	}
	if len(h.prices.created) != 1 {
		t.Fatalf("periods opened = %d, want 1", len(h.prices.created))
	}
}

func TestBranchCatalogService_SetPrice_AppliesTheDefaults(t *testing.T) {
	h := newPricingHarness(nil, testProductID)

	if _, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID,
		domain.NewProductPrice{Price: decimal.RequireFromString("8500.00")}); err != nil {
		t.Fatalf("SetPrice() = %v, want no error", err)
	}

	got := h.prices.created[0]
	if got.Currency != domain.DefaultCurrency {
		t.Errorf("currency = %q, want %q", got.Currency, domain.DefaultCurrency)
	}
	if !got.ValidFrom.Equal(fixedNow) {
		t.Errorf("valid_from = %v, want the injected clock %v", got.ValidFrom, fixedNow)
	}
	if got.MinPrice.Valid {
		t.Error("min_price is set; an omitted floor must stay NULL")
	}
}

func TestBranchCatalogService_SetPrice_Rejects(t *testing.T) {
	previousStart := fixedNow.Add(-72 * time.Hour)

	cases := []struct {
		name string
		open *domain.ProductPrice
		in   domain.NewProductPrice
	}{
		{
			name: "a floor above the price it floors",
			in: domain.NewProductPrice{
				Price:    decimal.RequireFromString("8500.00"),
				MinPrice: amount("9000.00"),
			},
		},
		{
			name: "a negative price",
			in:   domain.NewProductPrice{Price: decimal.RequireFromString("-1.00")},
		},
		{
			name: "a negative floor",
			in: domain.NewProductPrice{
				Price:    decimal.RequireFromString("8500.00"),
				MinPrice: amount("-0.01"),
			},
		},
		{
			// Postgres would round the third decimal away without saying so, and silent
			// rounding of money is a defect.
			name: "more decimals than NUMERIC(14,2) keeps",
			in:   domain.NewProductPrice{Price: decimal.RequireFromString("8500.555")},
		},
		{
			name: "more digits than NUMERIC(14,2) holds",
			in:   domain.NewProductPrice{Price: decimal.RequireFromString("1000000000000.00")},
		},
		{
			// Back-dating past the price it replaces would rewrite which price applied at a
			// moment already quoted, and would close a period before it opened.
			name: "a start before the period it replaces",
			open: &domain.ProductPrice{Price: decimal.RequireFromString("8500.00"), ValidFrom: previousStart},
			in: domain.NewProductPrice{
				Price:     decimal.RequireFromString("9100.00"),
				ValidFrom: previousStart.Add(-time.Hour),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPricingHarness(tc.open, testProductID)

			_, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID, tc.in)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("SetPrice() = %v, want %v", err, domain.ErrInvalidInput)
			}
			if len(h.prices.created) != 0 {
				t.Error("a period was opened for a rejected price")
			}
			if len(h.prices.closed) != 0 {
				t.Error("a period was closed for a rejected price; the old price must stay in force")
			}
		})
	}
}

// product_price and branch_product are branch-scoped, so a write without an active branch
// has no correct target — guessing one would price the wrong branch.
func TestBranchCatalogService_WritesRequireAnActiveBranch(t *testing.T) {
	accountWide := domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin}

	t.Run("price", func(t *testing.T) {
		h := newPricingHarness(nil, testProductID)
		_, err := h.service.SetPrice(context.Background(), accountWide, testProductID,
			domain.NewProductPrice{Price: decimal.RequireFromString("8500.00")})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("SetPrice() = %v, want %v", err, domain.ErrInvalidInput)
		}
		if len(h.prices.created) != 0 {
			t.Error("a period was opened without an active branch")
		}
	})

	t.Run("availability", func(t *testing.T) {
		h := newPricingHarness(nil, testProductID)
		_, err := h.service.SetAvailability(context.Background(), accountWide, testProductID,
			domain.BranchAvailability{IsActive: true})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("SetAvailability() = %v, want %v", err, domain.ErrInvalidInput)
		}
		if len(h.availability.saved) != 0 {
			t.Error("availability was saved without an active branch")
		}
	})
}

// The product read inside the tenant scope is what confines these writes to the account:
// the foreign key would accept another account's product on its own.
func TestBranchCatalogService_WritesRequireAProductInTheAccount(t *testing.T) {
	t.Run("price", func(t *testing.T) {
		h := newPricingHarness(nil)
		_, err := h.service.SetPrice(context.Background(), branchTenant(), testProductID,
			domain.NewProductPrice{Price: decimal.RequireFromString("8500.00")})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("SetPrice() = %v, want %v", err, domain.ErrNotFound)
		}
		if len(h.prices.created) != 0 {
			t.Error("a period was opened for a product outside the account")
		}
	})

	t.Run("availability", func(t *testing.T) {
		h := newPricingHarness(nil)
		_, err := h.service.SetAvailability(context.Background(), branchTenant(), testProductID,
			domain.BranchAvailability{IsActive: true})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("SetAvailability() = %v, want %v", err, domain.ErrNotFound)
		}
		if len(h.availability.saved) != 0 {
			t.Error("availability was saved for a product outside the account")
		}
	})
}

func TestBranchCatalogService_SetAvailability_DistinguishesUntrackedStockFromZero(t *testing.T) {
	cases := []struct {
		name      string
		stock     decimal.NullDecimal
		wantValid bool
		want      string
	}{
		{"untracked", decimal.NullDecimal{}, false, ""},
		{"zero", amount("0"), true, "0"},
		{"a real quantity", amount("250.50"), true, "250.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPricingHarness(nil, testProductID)
			if _, err := h.service.SetAvailability(context.Background(), branchTenant(), testProductID,
				domain.BranchAvailability{Stock: tc.stock, IsActive: true}); err != nil {
				t.Fatalf("SetAvailability() = %v, want no error", err)
			}

			got := h.availability.saved[0].Stock
			if got.Valid != tc.wantValid {
				t.Fatalf("stock valid = %v, want %v", got.Valid, tc.wantValid)
			}
			if tc.wantValid && got.Decimal.String() != tc.want {
				t.Errorf("stock = %s, want %s", got.Decimal.String(), tc.want)
			}
		})
	}
}

// Reads narrow to the active branch when the request selected one, and stay account-wide
// when it did not — which is how an admin compares branches.
func TestBranchCatalogService_ReadsFollowTheActiveBranch(t *testing.T) {
	accountWide := domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin}

	h := newPricingHarness(nil, testProductID)
	if _, err := h.service.ListPrices(context.Background(), branchTenant(), testProductID); err != nil {
		t.Fatalf("ListPrices() = %v, want no error", err)
	}
	if _, err := h.service.ListAvailability(context.Background(), branchTenant(), testProductID); err != nil {
		t.Fatalf("ListAvailability() = %v, want no error", err)
	}
	if len(h.prices.branch) != 1 || h.prices.branch[0] != testBranchID {
		t.Errorf("price read filtered by %v, want [%v]", h.prices.branch, testBranchID)
	}
	if len(h.availability.branch) != 1 || h.availability.branch[0] != testBranchID {
		t.Errorf("availability read filtered by %v, want [%v]", h.availability.branch, testBranchID)
	}

	wide := newPricingHarness(nil, testProductID)
	if _, err := wide.service.ListPrices(context.Background(), accountWide, testProductID); err != nil {
		t.Fatalf("ListPrices() = %v, want no error", err)
	}
	if len(wide.prices.branch) != 0 {
		t.Errorf("price read filtered by %v, want no branch filter", wide.prices.branch)
	}
}
