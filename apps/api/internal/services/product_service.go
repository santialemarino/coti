package services

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

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
	SetImage(ctx context.Context, q repository.Querier, accountID, id, imageID uuid.UUID) (*domain.Product, error)
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

// productAvailabilityWriter makes a new product available at the account's branches.
type productAvailabilityWriter interface {
	AddToActiveBranches(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID) ([]uuid.UUID, error)
}

// productInitialPriceWriter opens a new product's first price period at several branches.
type productInitialPriceWriter interface {
	CreateAtBranches(ctx context.Context, q repository.Querier, accountID, productID uuid.UUID, branchIDs []uuid.UUID, userID *uuid.UUID, in domain.NewProductPrice) error
}

// tenantTxRunner is the database surface a tenant-scoped use case needs: one transaction
// carrying the account scope that row level security reads.
type tenantTxRunner interface {
	InTenantTx(ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error) error
}

// ProductService owns the account-level catalog: products, their synonyms, and the
// alternative links between them.
type ProductService struct {
	db            tenantTxRunner
	products      productRepository
	synonyms      productSynonymRepository
	alternatives  productAlternativeRepository
	availability  productAvailabilityWriter
	prices        productInitialPriceWriter
	cfg           config.CatalogConfig
	imageStorage  domain.ObjectStorage
	imageMaxBytes int64
	now           func() time.Time
}

// NewProductService builds a ProductService.
func NewProductService(
	db tenantTxRunner, products productRepository, synonyms productSynonymRepository,
	alternatives productAlternativeRepository, availability productAvailabilityWriter,
	prices productInitialPriceWriter, cfg config.CatalogConfig,
) *ProductService {
	return &ProductService{
		db: db, products: products, synonyms: synonyms, alternatives: alternatives,
		availability: availability, prices: prices, cfg: cfg, now: time.Now,
	}
}

// WithImageStorage enables product-image uploads and public reads.
func (s *ProductService) WithImageStorage(storage domain.ObjectStorage, maxBytes int64) *ProductService {
	s.imageStorage = storage
	s.imageMaxBytes = maxBytes
	return s
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

// CreateProduct adds a catalog item to the account and makes it available at every active
// branch, priced there when an initial price came with it, all in one transaction.
//
// Returns domain.ErrConflict when the code is already taken within the account, and
// domain.ErrInvalidInput when the name is blank once trimmed or the price does not hold up.
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
	if in.InitialPrice != nil {
		if err := validatePriceAmounts(*in.InitialPrice); err != nil {
			return nil, err
		}
		price := *in.InitialPrice
		price.Currency = domain.DefaultCurrency
		price.ValidFrom = s.now()
		in.InitialPrice = &price
	}

	var product *domain.Product
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		product, createErr = s.products.Create(ctx, q, tenant.AccountID, in)
		if createErr != nil {
			return createErr
		}
		branchIDs, addErr := s.availability.AddToActiveBranches(ctx, q, tenant.AccountID, product.ID)
		if addErr != nil {
			return addErr
		}
		if in.InitialPrice == nil || len(branchIDs) == 0 {
			return nil
		}
		return s.prices.CreateAtBranches(ctx, q, tenant.AccountID, product.ID, branchIDs,
			&tenant.UserID, *in.InitialPrice)
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

// UploadImage validates, stores, and assigns one product's primary photo.
func (s *ProductService) UploadImage(
	ctx context.Context, tenant domain.Tenant, productID uuid.UUID, file domain.ProductImageUpload,
) (*domain.Product, error) {
	if s.imageStorage == nil {
		return nil, fmt.Errorf("product image storage is unavailable")
	}
	file.ContentType = normalizeContentType(file.ContentType)
	if file.Size <= 0 {
		return nil, fmt.Errorf("%w: the product image is empty", domain.ErrInvalidInput)
	}
	if file.Size > s.imageMaxBytes {
		return nil, fmt.Errorf("%w: the product image exceeds %d bytes", domain.ErrTooLarge, s.imageMaxBytes)
	}
	if file.ContentType != "image/png" && file.ContentType != "image/jpeg" && file.ContentType != "image/webp" {
		return nil, domain.WithCode(domain.CodeUnsupportedFileType,
			fmt.Errorf("%w: product images must be PNG, JPEG, or WebP", domain.ErrInvalidInput))
	}

	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		_, err := s.products.GetByID(ctx, q, tenant.AccountID, productID)
		return err
	}); err != nil {
		return nil, err
	}

	content := bufio.NewReader(file.Content)
	head, err := content.Peek(512)
	if err != nil && len(head) == 0 {
		return nil, fmt.Errorf("%w: read product image header", domain.ErrInvalidInput)
	}
	if detected := http.DetectContentType(head); detected != file.ContentType {
		return nil, domain.WithCode(domain.CodeUnsupportedFileType,
			fmt.Errorf("%w: image bytes are %s, not %s", domain.ErrInvalidInput, detected, file.ContentType))
	}

	imageID := uuid.New()
	if err := s.imageStorage.Upload(ctx, productImageKey(tenant.AccountID, productID, imageID),
		file.ContentType, content); err != nil {
		return nil, err
	}

	var product *domain.Product
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var updateErr error
		product, updateErr = s.products.SetImage(ctx, q, tenant.AccountID, productID, imageID)
		return updateErr
	}); err != nil {
		return nil, err
	}
	return product, nil
}

// DownloadImage returns one public product image by its unguessable identifier.
func (s *ProductService) DownloadImage(
	ctx context.Context, accountID, productID, imageID uuid.UUID,
) (*domain.StoredObject, error) {
	if s.imageStorage == nil {
		return nil, fmt.Errorf("product image storage is unavailable")
	}
	return s.imageStorage.Download(ctx, productImageKey(accountID, productID, imageID))
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

func productImageKey(accountID, productID, imageID uuid.UUID) string {
	return path.Join("accounts", accountID.String(), "products", productID.String(), imageID.String())
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
