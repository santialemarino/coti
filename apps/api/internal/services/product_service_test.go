package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The catalog's decisions live in the service — input normalization, the page-size clamp,
// and the checks that stop a link from crossing accounts — so they are tested against
// in-memory fakes. The SQL and the row level security policy are covered by the
// integration tests in internal/repository.

var (
	testProductID     = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	testAlternativeID = uuid.MustParse("44444444-4444-4444-8444-444444444444")
)

func testCatalogConfig() config.CatalogConfig {
	return config.CatalogConfig{DefaultPageSize: 50, MaxPageSize: 200}
}

// fakeProducts records what the service asked for and answers with whatever the test set.
type fakeProducts struct {
	known   map[uuid.UUID]domain.Product
	filters []domain.ProductFilter
	created []domain.NewProduct
	updated []domain.ProductUpdate
	deleted []uuid.UUID
	locked  []uuid.UUID
}

func newFakeProducts(ids ...uuid.UUID) *fakeProducts {
	known := make(map[uuid.UUID]domain.Product, len(ids))
	for _, id := range ids {
		known[id] = domain.Product{ID: id, AccountID: testAccountID, CanonicalName: "Cemento"}
	}
	return &fakeProducts{known: known}
}

func (f *fakeProducts) List(
	_ context.Context, _ repository.Querier, _ uuid.UUID, filter domain.ProductFilter,
) (domain.ProductPage, error) {
	f.filters = append(f.filters, filter)
	return domain.ProductPage{Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (f *fakeProducts) GetByID(
	_ context.Context, _ repository.Querier, _, id uuid.UUID,
) (*domain.Product, error) {
	p, ok := f.known[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &p, nil
}

func (f *fakeProducts) GetByIDForUpdate(
	ctx context.Context, q repository.Querier, accountID, id uuid.UUID,
) (*domain.Product, error) {
	f.locked = append(f.locked, id)
	return f.GetByID(ctx, q, accountID, id)
}

func (f *fakeProducts) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewProduct,
) (*domain.Product, error) {
	f.created = append(f.created, in)
	return &domain.Product{ID: uuid.New(), AccountID: accountID, CanonicalName: in.CanonicalName}, nil
}

func (f *fakeProducts) Update(
	_ context.Context, _ repository.Querier, accountID, id uuid.UUID, in domain.ProductUpdate,
) (*domain.Product, error) {
	f.updated = append(f.updated, in)
	return &domain.Product{ID: id, AccountID: accountID, CanonicalName: in.CanonicalName}, nil
}

func (f *fakeProducts) Delete(_ context.Context, _ repository.Querier, _, id uuid.UUID) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeSynonyms struct {
	created []domain.ProductSynonym
	deleted []uuid.UUID
}

func (f *fakeSynonyms) Delete(_ context.Context, _ repository.Querier, _, _, id uuid.UUID) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeSynonyms) List(
	_ context.Context, _ repository.Querier, _, productID uuid.UUID,
) ([]domain.ProductSynonym, error) {
	return []domain.ProductSynonym{{ProductID: productID, Term: "portland"}}, nil
}

func (f *fakeSynonyms) Create(
	_ context.Context, _ repository.Querier, accountID, productID uuid.UUID, term string,
	source domain.SynonymSource,
) (*domain.ProductSynonym, error) {
	s := domain.ProductSynonym{
		ID: uuid.New(), AccountID: accountID, ProductID: productID, Term: term, Source: source,
	}
	f.created = append(f.created, s)
	return &s, nil
}

type fakeAlternatives struct {
	directions []domain.AlternativeDirection
	created    []domain.ProductAlternative
}

func (f *fakeAlternatives) List(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, direction domain.AlternativeDirection,
) ([]domain.ProductAlternativeView, error) {
	f.directions = append(f.directions, direction)
	return nil, nil
}

func (f *fakeAlternatives) Create(
	_ context.Context, _ repository.Querier, accountID, baseProductID, alternativeProductID uuid.UUID,
	alternativeType domain.ProductAlternativeType,
) (*domain.ProductAlternative, error) {
	link := domain.ProductAlternative{
		ID: uuid.New(), AccountID: accountID, BaseProductID: baseProductID,
		AlternativeProductID: alternativeProductID, Type: alternativeType,
	}
	f.created = append(f.created, link)
	return &link, nil
}

func (f *fakeAlternatives) Delete(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID) error {
	return nil
}

type catalogHarness struct {
	service      *ProductService
	db           *fakeDB
	products     *fakeProducts
	synonyms     *fakeSynonyms
	alternatives *fakeAlternatives
}

func newCatalogHarness(known ...uuid.UUID) *catalogHarness {
	h := &catalogHarness{
		db:           &fakeDB{},
		products:     newFakeProducts(known...),
		synonyms:     &fakeSynonyms{},
		alternatives: &fakeAlternatives{},
	}
	h.service = NewProductService(h.db, h.products, h.synonyms, h.alternatives, testCatalogConfig())
	return h
}

func testTenant() domain.Tenant {
	return domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleAdmin}
}

func ptr(s string) *string { return &s }

func TestProductService_CreateProduct_NormalizesText(t *testing.T) {
	h := newCatalogHarness()

	if _, err := h.service.CreateProduct(context.Background(), testTenant(), domain.NewProduct{
		Code:          ptr("   "),
		CanonicalName: "  Cemento Portland 50kg  ",
		Description:   ptr("  bolsa de 50 kilos "),
		Unit:          ptr(""),
	}); err != nil {
		t.Fatalf("CreateProduct() = %v, want no error", err)
	}

	if len(h.products.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(h.products.created))
	}
	got := h.products.created[0]
	if got.CanonicalName != "Cemento Portland 50kg" {
		t.Errorf("canonical_name = %q, want %q", got.CanonicalName, "Cemento Portland 50kg")
	}
	// An empty code has to reach the database as NULL: the unique index is partial on NOT
	// NULL, so two empty strings would collide where two NULLs do not.
	if got.Code != nil {
		t.Errorf("code = %q, want nil", *got.Code)
	}
	if got.Unit != nil {
		t.Errorf("unit = %q, want nil", *got.Unit)
	}
	if got.Description == nil || *got.Description != "bolsa de 50 kilos" {
		t.Errorf("description = %v, want %q", got.Description, "bolsa de 50 kilos")
	}
}

func TestProductService_CreateProduct_RejectsABlankName(t *testing.T) {
	h := newCatalogHarness()

	_, err := h.service.CreateProduct(context.Background(), testTenant(), domain.NewProduct{
		CanonicalName: "   ",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateProduct() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.products.created) != 0 {
		t.Error("Create was called for a blank name; the service must reject it first")
	}
}

func TestProductService_ListProducts_ResolvesThePageWindow(t *testing.T) {
	cases := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{"omitted limit falls back to the default", 0, 0, 50, 0},
		{"a limit past the cap is clamped", 5000, 0, 200, 0},
		{"a limit inside the cap is kept", 25, 40, 25, 40},
		{"a negative offset is floored at zero", 10, -5, 10, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCatalogHarness()
			page, err := h.service.ListProducts(context.Background(), testTenant(), domain.ProductFilter{
				Limit:  tc.limit,
				Offset: tc.offset,
			})
			if err != nil {
				t.Fatalf("ListProducts() = %v, want no error", err)
			}
			if len(h.products.filters) != 1 {
				t.Fatalf("List called %d times, want 1", len(h.products.filters))
			}
			if got := h.products.filters[0].Limit; got != tc.wantLimit {
				t.Errorf("limit reaching the repository = %d, want %d", got, tc.wantLimit)
			}
			if got := h.products.filters[0].Offset; got != tc.wantOffset {
				t.Errorf("offset reaching the repository = %d, want %d", got, tc.wantOffset)
			}
			if page.Limit != tc.wantLimit {
				t.Errorf("page.Limit = %d, want %d", page.Limit, tc.wantLimit)
			}
		})
	}
}

// Every read and write runs inside a transaction scoped to the caller's account, which is
// what makes the row level security policy apply at all.
func TestProductService_ScopesEveryCallToTheCallersAccount(t *testing.T) {
	h := newCatalogHarness(testProductID)
	ctx := context.Background()
	tenant := testTenant()

	if _, err := h.service.GetProduct(ctx, tenant, testProductID); err != nil {
		t.Fatalf("GetProduct() = %v, want no error", err)
	}
	if err := h.service.DeleteProduct(ctx, tenant, testProductID); err != nil {
		t.Fatalf("DeleteProduct() = %v, want no error", err)
	}
	if err := h.service.RemoveSynonym(ctx, tenant, testProductID, testAlternativeID); err != nil {
		t.Fatalf("RemoveSynonym() = %v, want no error", err)
	}

	if len(h.db.scopes) != 3 {
		t.Fatalf("tenant-scoped transactions opened = %d, want 3", len(h.db.scopes))
	}
	for _, accountID := range h.db.scopes {
		if accountID != testAccountID {
			t.Errorf("transaction scoped to %v, want %v", accountID, testAccountID)
		}
	}
	if len(h.products.deleted) != 1 || h.products.deleted[0] != testProductID {
		t.Errorf("products deleted = %v, want [%v]", h.products.deleted, testProductID)
	}
	if len(h.synonyms.deleted) != 1 || h.synonyms.deleted[0] != testAlternativeID {
		t.Errorf("synonyms deleted = %v, want [%v]", h.synonyms.deleted, testAlternativeID)
	}
}

func TestProductService_AddSynonym_DefaultsTheSourceToManual(t *testing.T) {
	h := newCatalogHarness(testProductID)

	if _, err := h.service.AddSynonym(context.Background(), testTenant(), testProductID,
		"  portland  ", ""); err != nil {
		t.Fatalf("AddSynonym() = %v, want no error", err)
	}

	if len(h.synonyms.created) != 1 {
		t.Fatalf("Create called %d times, want 1", len(h.synonyms.created))
	}
	got := h.synonyms.created[0]
	if got.Term != "portland" {
		t.Errorf("term = %q, want %q", got.Term, "portland")
	}
	if got.Source != domain.SynonymSourceManual {
		t.Errorf("source = %q, want %q", got.Source, domain.SynonymSourceManual)
	}
}

// The product is read inside the tenant scope before the synonym is written, because the
// foreign key would happily accept another account's product.
func TestProductService_AddSynonym_RequiresAProductInTheAccount(t *testing.T) {
	h := newCatalogHarness()

	_, err := h.service.AddSynonym(context.Background(), testTenant(), testProductID, "portland",
		domain.SynonymSourceManual)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AddSynonym() = %v, want %v", err, domain.ErrNotFound)
	}
	if len(h.synonyms.created) != 0 {
		t.Error("Create was called for a product outside the account")
	}
}

func TestProductService_AddAlternative_RejectsAProductAsItsOwnAlternative(t *testing.T) {
	h := newCatalogHarness(testProductID)

	_, err := h.service.AddAlternative(context.Background(), testTenant(), testProductID,
		testProductID, domain.ProductAlternativeEquivalent)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AddAlternative() = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(h.alternatives.created) != 0 {
		t.Error("Create was called for a self-reference")
	}
}

// Both ends are checked: the far one is the id an attacker controls, and the foreign key
// alone does not confine it to the account.
func TestProductService_AddAlternative_RequiresBothProductsInTheAccount(t *testing.T) {
	cases := []struct {
		name  string
		known []uuid.UUID
	}{
		{"base outside the account", []uuid.UUID{testAlternativeID}},
		{"alternative outside the account", []uuid.UUID{testProductID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCatalogHarness(tc.known...)
			_, err := h.service.AddAlternative(context.Background(), testTenant(), testProductID,
				testAlternativeID, domain.ProductAlternativeEconomy)
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("AddAlternative() = %v, want %v", err, domain.ErrNotFound)
			}
			if len(h.alternatives.created) != 0 {
				t.Error("Create was called with a product outside the account")
			}
		})
	}
}

func TestProductService_AddAlternative_LinksBothProducts(t *testing.T) {
	h := newCatalogHarness(testProductID, testAlternativeID)

	link, err := h.service.AddAlternative(context.Background(), testTenant(), testProductID,
		testAlternativeID, domain.ProductAlternativePremium)
	if err != nil {
		t.Fatalf("AddAlternative() = %v, want no error", err)
	}
	if link.BaseProductID != testProductID || link.AlternativeProductID != testAlternativeID {
		t.Errorf("link = %v -> %v, want %v -> %v", link.BaseProductID, link.AlternativeProductID,
			testProductID, testAlternativeID)
	}
	if link.Type != domain.ProductAlternativePremium {
		t.Errorf("type = %q, want %q", link.Type, domain.ProductAlternativePremium)
	}
}

func TestProductService_ListAlternatives_DefaultsToOutgoing(t *testing.T) {
	cases := []struct {
		name      string
		requested domain.AlternativeDirection
		want      domain.AlternativeDirection
	}{
		{"omitted", "", domain.AlternativeDirectionOutgoing},
		{"outgoing", domain.AlternativeDirectionOutgoing, domain.AlternativeDirectionOutgoing},
		{"incoming", domain.AlternativeDirectionIncoming, domain.AlternativeDirectionIncoming},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCatalogHarness(testProductID)
			if _, err := h.service.ListAlternatives(context.Background(), testTenant(),
				testProductID, tc.requested); err != nil {
				t.Fatalf("ListAlternatives() = %v, want no error", err)
			}
			if len(h.alternatives.directions) != 1 {
				t.Fatalf("List called %d times, want 1", len(h.alternatives.directions))
			}
			if got := h.alternatives.directions[0]; got != tc.want {
				t.Errorf("direction reaching the repository = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProductService_UpdateProduct_LeavesTheActiveFlagAloneWhenOmitted(t *testing.T) {
	h := newCatalogHarness(testProductID)

	if _, err := h.service.UpdateProduct(context.Background(), testTenant(), testProductID,
		domain.ProductUpdate{CanonicalName: "Cemento Portland 50kg"}); err != nil {
		t.Fatalf("UpdateProduct() = %v, want no error", err)
	}

	if len(h.products.updated) != 1 {
		t.Fatalf("Update called %d times, want 1", len(h.products.updated))
	}
	if h.products.updated[0].IsActive != nil {
		t.Error("is_active reached the repository set; an edit must not revive a deactivated item")
	}
}
