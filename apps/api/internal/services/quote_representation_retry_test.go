package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestQuoteRepresentationService_RetryPreservesFrozenRecommendationAfterCatalogChange(t *testing.T) {
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	source := representationSourceFixture()
	source.Version.ID = uuid.New()
	source.Quote.ID = uuid.New()
	source.Quote.CurrentVersionID = &source.Version.ID
	source.Quote.CurrentStatus = domain.QuoteStatusQuoted
	itemID, baseID, alternativeID := source.Items[0].ID, *source.Items[0].ProductID, uuid.New()
	family, subgroup, unit := uuid.New(), uuid.New(), "bolsa"
	kind := domain.ProductAlternativeEconomy
	alternative := &source.Alternatives[itemID][0]
	alternative.ID = uuid.New()
	alternative.ProductID = &alternativeID
	alternative.Type = domain.QuoteItemAlternativeTypeProduct
	alternative.RecommendationType = &kind
	products := map[uuid.UUID]domain.Product{
		baseID:        {ID: baseID, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
		alternativeID: {ID: alternativeID, FamilyID: &family, SubgroupID: &subgroup, Unit: &unit},
	}
	quotes := &representationRetryQuotes{fakePublicQuoteQuotes: fakePublicQuoteQuotes{quote: &source.Quote}, source: &source}
	representations := &representationRetryRepository{source: &source}
	renderer := &representationRetryRenderer{}
	service := NewQuoteRepresentationService(&fakePublicQuoteDB{}, representations, quotes,
		&fakeObjectStorage{}, nil, renderer, time.Minute, nil, nil).
		WithRecommendationProducts(fakeRecommendationProducts{products: products})

	if _, err := service.Ensure(ctx, tenant, source.Quote.ID); !errors.Is(err, domain.ErrRepresentationUnavailable) {
		t.Fatalf("first Ensure() = %v, want rendering failure", err)
	}
	if !source.Version.IsImmutable || representations.created != nil {
		t.Fatal("render failure must leave a frozen version without a representation")
	}
	candidate := products[alternativeID]
	otherSubgroup := uuid.New()
	candidate.SubgroupID = &otherSubgroup
	products[alternativeID] = candidate

	result, err := service.Ensure(ctx, tenant, source.Quote.ID)
	if err != nil {
		t.Fatalf("retry Ensure() = %v, want the already approved frozen offer", err)
	}
	if len(result.Representation.Payload.Items[0].Alternatives) != 1 {
		t.Fatal("retry changed the alternatives approved before the version was frozen")
	}
	if quotes.frozenApprovalWrites != 0 {
		t.Fatalf("retry attempted %d approval writes on a frozen version", quotes.frozenApprovalWrites)
	}
}

type representationRetryQuotes struct {
	fakePublicQuoteQuotes
	source               *domain.QuoteRepresentationSource
	frozenApprovalWrites int
}

func (f *representationRetryQuotes) GetCurrentVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID,
) (*domain.QuoteVersion, error) {
	version := f.source.Version
	return &version, nil
}

func (f *representationRetryQuotes) FreezeVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID,
) (*domain.QuoteVersion, error) {
	f.source.Version.IsImmutable = true
	if f.source.Version.FrozenAt == nil {
		now := time.Now().UTC()
		f.source.Version.FrozenAt = &now
	}
	version := f.source.Version
	return &version, nil
}

func (f *representationRetryQuotes) UnapproveAlternatives(_ context.Context, _ repository.Querier,
	_, _ uuid.UUID, ids []uuid.UUID,
) error {
	if len(ids) == 0 {
		return nil
	}
	if f.source.Version.IsImmutable {
		f.frozenApprovalWrites++
		return domain.ErrConflict
	}
	return nil
}

type representationRetryRepository struct {
	quoteRepresentationRepository
	source  *domain.QuoteRepresentationSource
	created *domain.QuoteRepresentation
}

func (f *representationRetryRepository) GetByVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID,
) (*domain.QuoteRepresentation, error) {
	if f.created == nil {
		return nil, domain.ErrNotFound
	}
	return f.created, nil
}

func (f *representationRetryRepository) LoadSource(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID,
) (*domain.QuoteRepresentationSource, error) {
	source := *f.source
	source.Alternatives = make(map[uuid.UUID][]domain.QuoteItemAlternative, len(f.source.Alternatives))
	for itemID, alternatives := range f.source.Alternatives {
		source.Alternatives[itemID] = append([]domain.QuoteItemAlternative(nil), alternatives...)
	}
	return &source, nil
}

func (f *representationRetryRepository) Create(_ context.Context, _ repository.Querier,
	accountID uuid.UUID, in domain.NewQuoteRepresentation,
) (*domain.QuoteRepresentation, error) {
	f.created = &domain.QuoteRepresentation{ID: in.ID, AccountID: accountID,
		VersionID: in.VersionID, Payload: in.Payload, PDFStorageKey: in.PDFStorageKey}
	return f.created, nil
}

type representationRetryRenderer struct{ calls int }

func (f *representationRetryRenderer) Render(domain.QuoteRepresentationPayload, *domain.BrandLogo) ([]byte, error) {
	f.calls++
	if f.calls == 1 {
		return nil, errors.New("temporary PDF render failure")
	}
	return []byte("pdf"), nil
}
