package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// productRepository is the catalog persistence surface the service needs. Defined here,
// in the consumer, so a test can fake it without a database.
type productRepository interface {
	List(ctx context.Context, q repository.Querier, accountID uuid.UUID, f domain.ProductFilter) (domain.ProductPage, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) (*domain.Product, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewProduct) (*domain.Product, error)
	Update(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, in domain.ProductUpdate) (*domain.Product, error)
	Delete(ctx context.Context, q repository.Querier, accountID, id uuid.UUID) error
}

// productSynonymRepository is the synonym persistence surface.
type productSynonymRepository interface {
	List(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID) ([]domain.ProductSynonym, error)
	Create(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, term string, source domain.SynonymSource) (*domain.ProductSynonym, error)
	Delete(ctx context.Context, q repository.Querier, accountID, productID, id uuid.UUID) error
}

// productAlternativeRepository is the alternative-link persistence surface.
type productAlternativeRepository interface {
	List(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, direction domain.AlternativeDirection) ([]domain.ProductAlternativeView, error)
	Create(ctx context.Context, q repository.Querier, accountID, baseProductID, alternativeProductID uuid.UUID, alternativeType domain.ProductAlternativeType) (*domain.ProductAlternative, error)
	Delete(ctx context.Context, q repository.Querier, accountID, productID, id uuid.UUID) error
}

// tenantTxRunner is the database surface a tenant-scoped use case needs: one transaction
// carrying the account scope that row level security reads.
type tenantTxRunner interface {
	InTenantTx(ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error) error
}

// ProductService owns the account-level catalog: products, their synonyms, and the
// alternative links between them.
type ProductService struct {
	db           tenantTxRunner
	products     productRepository
	synonyms     productSynonymRepository
	alternatives productAlternativeRepository
	cfg          config.CatalogConfig
}

// NewProductService builds a ProductService.
func NewProductService(
	db tenantTxRunner, products productRepository, synonyms productSynonymRepository,
	alternatives productAlternativeRepository, cfg config.CatalogConfig,
) *ProductService {
	return &ProductService{
		db: db, products: products, synonyms: synonyms, alternatives: alternatives, cfg: cfg,
	}
}

// ListProducts returns one page of the account's catalog, with the page size resolved
// against the configured default and cap.
func (s *ProductService) ListProducts(
	ctx context.Context, tenant domain.Tenant, filter domain.ProductFilter,
) (domain.ProductPage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Limit = s.resolveLimit(filter.Limit)
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var page domain.ProductPage
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		page, listErr = s.products.List(ctx, q, tenant.AccountID, filter)
		return listErr
	})
	return page, err
}

// GetProduct loads one catalog item. Returns domain.ErrNotFound when it does not exist or
// belongs to another account — the two are indistinguishable on purpose.
func (s *ProductService) GetProduct(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID,
) (*domain.Product, error) {
	var product *domain.Product
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var getErr error
		product, getErr = s.products.GetByID(ctx, q, tenant.AccountID, id)
		return getErr
	})
	if err != nil {
		return nil, err
	}
	return product, nil
}

// CreateProduct adds a catalog item to the account.
//
// Returns domain.ErrConflict when the code is already taken within the account, and
// domain.ErrInvalidInput when the name is blank once trimmed.
func (s *ProductService) CreateProduct(
	ctx context.Context, tenant domain.Tenant, in domain.NewProduct,
) (*domain.Product, error) {
	name, err := requiredText(in.CanonicalName, "canonical_name")
	if err != nil {
		return nil, err
	}
	in.CanonicalName = name
	in.Code = optionalText(in.Code)
	in.Description = optionalText(in.Description)
	in.Unit = optionalText(in.Unit)

	var product *domain.Product
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		product, createErr = s.products.Create(ctx, q, tenant.AccountID, in)
		return createErr
	}); err != nil {
		return nil, err
	}
	return product, nil
}

// UpdateProduct replaces the item's editable attributes and returns the stored row.
func (s *ProductService) UpdateProduct(
	ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.ProductUpdate,
) (*domain.Product, error) {
	name, err := requiredText(in.CanonicalName, "canonical_name")
	if err != nil {
		return nil, err
	}
	in.CanonicalName = name
	in.Code = optionalText(in.Code)
	in.Description = optionalText(in.Description)
	in.Unit = optionalText(in.Unit)

	var product *domain.Product
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var updateErr error
		product, updateErr = s.products.Update(ctx, q, tenant.AccountID, id, in)
		return updateErr
	}); err != nil {
		return nil, err
	}
	return product, nil
}

// DeleteProduct deactivates the item. The row survives because quote history and price
// history point at it; only new quotes stop matching it.
func (s *ProductService) DeleteProduct(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.products.Delete(ctx, q, tenant.AccountID, id)
	})
}

// ListSynonyms returns the product's synonyms.
func (s *ProductService) ListSynonyms(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID,
) ([]domain.ProductSynonym, error) {
	var synonyms []domain.ProductSynonym
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var listErr error
		synonyms, listErr = s.synonyms.List(ctx, q, tenant.AccountID, productID)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return synonyms, nil
}

// AddSynonym attaches a colloquial term to a product. Returns domain.ErrConflict when the
// product already carries it.
//
// Reading the product inside the tenant scope first is load-bearing: foreign keys are
// checked with row level security bypassed, so another account's id would link fine.
func (s *ProductService) AddSynonym(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID, term string,
	source domain.SynonymSource,
) (*domain.ProductSynonym, error) {
	trimmed, err := requiredText(term, "term")
	if err != nil {
		return nil, err
	}
	if source == "" {
		source = domain.SynonymSourceManual
	}

	var synonym *domain.ProductSynonym
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var createErr error
		synonym, createErr = s.synonyms.Create(ctx, q, tenant.AccountID, productID, trimmed, source)
		return createErr
	}); err != nil {
		return nil, err
	}
	return synonym, nil
}

// RemoveSynonym detaches a term from a product.
func (s *ProductService) RemoveSynonym(
	ctx context.Context, tenant domain.Tenant, productID, synonymID uuid.UUID,
) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.synonyms.Delete(ctx, q, tenant.AccountID, productID, synonymID)
	})
}

// ListAlternatives returns the product's alternative links from the requested end of the
// relation: OUTGOING is what can be offered instead of it, INCOMING what it stands in for.
func (s *ProductService) ListAlternatives(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID,
	direction domain.AlternativeDirection,
) ([]domain.ProductAlternativeView, error) {
	if direction == "" {
		direction = domain.AlternativeDirectionOutgoing
	}

	var views []domain.ProductAlternativeView
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, productID); getErr != nil {
			return getErr
		}
		var listErr error
		views, listErr = s.alternatives.List(ctx, q, tenant.AccountID, productID, direction)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// AddAlternative links a base product to an alternative. A product cannot be its own.
//
// Both ends are read inside the tenant scope first: a foreign key alone would accept
// another account's product, since constraint checks bypass row level security.
func (s *ProductService) AddAlternative(
	ctx context.Context, tenant domain.Tenant, baseProductID, alternativeProductID uuid.UUID,
	alternativeType domain.ProductAlternativeType,
) (*domain.ProductAlternative, error) {
	if baseProductID == alternativeProductID {
		return nil, fmt.Errorf("%w: a product cannot be its own alternative", domain.ErrInvalidInput)
	}

	var link *domain.ProductAlternative
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, baseProductID); getErr != nil {
			return getErr
		}
		if _, getErr := s.products.GetByID(ctx, q, tenant.AccountID, alternativeProductID); getErr != nil {
			return getErr
		}
		var createErr error
		link, createErr = s.alternatives.Create(ctx, q, tenant.AccountID, baseProductID,
			alternativeProductID, alternativeType)
		return createErr
	}); err != nil {
		return nil, err
	}
	return link, nil
}

// RemoveAlternative drops one link, addressed from either of the two products it joins.
func (s *ProductService) RemoveAlternative(
	ctx context.Context, tenant domain.Tenant, productID, alternativeID uuid.UUID,
) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.alternatives.Delete(ctx, q, tenant.AccountID, productID, alternativeID)
	})
}

// resolveLimit clamps a requested page size to the configured default and cap.
func (s *ProductService) resolveLimit(requested int) int {
	if requested <= 0 {
		return s.cfg.DefaultPageSize
	}
	if requested > s.cfg.MaxPageSize {
		return s.cfg.MaxPageSize
	}
	return requested
}

// requiredText trims a mandatory field and rejects it when nothing is left, so a value of
// spaces cannot pass a min-length check.
func requiredText(raw, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s cannot be blank", domain.ErrInvalidInput, field)
	}
	return trimmed, nil
}

// optionalText trims a nullable field and collapses an empty result to NULL. The collapse
// matters for product.code: its unique index is partial on NOT NULL, so two empty strings
// would collide where two NULLs do not.
func optionalText(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
