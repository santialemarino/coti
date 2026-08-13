package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// fakeCatalogEmbedding hands back staged pages in order and records how it was asked for them.
type fakeCatalogEmbedding struct {
	pages      [][]domain.ProductEmbeddingInput
	reads      int
	cursors    []uuid.UUID
	refreshAll []bool
	written    [][]domain.ProductEmbedding
	writeErr   error
}

func (f *fakeCatalogEmbedding) ListPendingEmbedding(
	_ context.Context, _ repository.Querier, _, cursor uuid.UUID, refreshAll bool, _ int,
) ([]domain.ProductEmbeddingInput, error) {
	f.cursors = append(f.cursors, cursor)
	f.refreshAll = append(f.refreshAll, refreshAll)
	if f.reads >= len(f.pages) {
		return nil, nil
	}
	page := f.pages[f.reads]
	f.reads++
	return page, nil
}

func (f *fakeCatalogEmbedding) SetEmbeddings(
	_ context.Context, _ repository.Querier, _ uuid.UUID, embeddings []domain.ProductEmbedding,
) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.written = append(f.written, embeddings)
	return len(embeddings), nil
}

func productPage(names ...string) []domain.ProductEmbeddingInput {
	page := make([]domain.ProductEmbeddingInput, len(names))
	for i, name := range names {
		page[i] = domain.ProductEmbeddingInput{ProductID: uuid.New(), CanonicalName: name}
	}
	return page
}

// The backfill reads a page, embeds it outside any transaction, writes it back, and moves the
// cursor past it. Paging by id is what stops a full refresh re-reading the rows it just wrote.
func TestCatalogEmbeddingService_PagesThroughTheCatalog(t *testing.T) {
	first := productPage("Cemento Portland 50kg", "Cal hidratada")
	second := productPage("Arena fina")
	products := &fakeCatalogEmbedding{pages: [][]domain.ProductEmbeddingInput{first, second}}
	embedder := &fakeEmbedder{}
	service := NewCatalogEmbeddingService(&fakeDB{}, products, embedder, testSearchConfig())

	report, err := service.Backfill(context.Background(), domain.Tenant{AccountID: testAccountID}, false)
	if err != nil {
		t.Fatalf("Backfill() = %v, want no error", err)
	}
	if report.Embedded != 3 || report.Rounds != 2 {
		t.Errorf("report = %+v, want 3 embedded over 2 rounds", report)
	}

	// A third read is what tells the run the catalog is done.
	wantCursors := []uuid.UUID{uuid.Nil, first[1].ProductID, second[0].ProductID}
	if len(products.cursors) != len(wantCursors) {
		t.Fatalf("reads = %d, want %d", len(products.cursors), len(wantCursors))
	}
	for i, want := range wantCursors {
		if products.cursors[i] != want {
			t.Errorf("cursor on read %d = %v, want %v", i, products.cursors[i], want)
		}
	}
	if len(embedder.calls) != 2 || len(embedder.calls[0]) != 2 {
		t.Errorf("embedder calls = %v, want one per page", embedder.calls)
	}
	if embedder.calls[0][0] != "Cemento Portland 50kg" {
		t.Errorf("embedded text = %q, want the product's own text", embedder.calls[0][0])
	}
}

func TestCatalogEmbeddingService_PassesRefreshAllThrough(t *testing.T) {
	products := &fakeCatalogEmbedding{}
	service := NewCatalogEmbeddingService(&fakeDB{}, products, &fakeEmbedder{}, testSearchConfig())

	if _, err := service.Backfill(context.Background(),
		domain.Tenant{AccountID: testAccountID}, true); err != nil {
		t.Fatalf("Backfill() = %v, want no error", err)
	}
	if len(products.refreshAll) != 1 || !products.refreshAll[0] {
		t.Errorf("refreshAll seen by the repository = %v, want a single true", products.refreshAll)
	}
}

// lateFailingDB commits the read and then fails the write, which is what a round rolled back at
// commit time looks like from the service's side.
type lateFailingDB struct {
	calls int
	err   error
}

func (d *lateFailingDB) InTenantTx(
	_ context.Context, _ domain.Tenant, fn func(repository.Querier) error,
) error {
	d.calls++
	if err := fn(nil); err != nil {
		return err
	}
	if d.calls > 1 {
		return d.err
	}
	return nil
}

// A round that stored nothing must count nothing, or an operator reads the report as a catalog
// that is embedded when it is not.
func TestCatalogEmbeddingService_CountsNothingForAFailedRound(t *testing.T) {
	wantErr := errors.New("round failed")

	t.Run("the write is refused", func(t *testing.T) {
		products := &fakeCatalogEmbedding{
			pages:    [][]domain.ProductEmbeddingInput{productPage("Cemento Portland 50kg")},
			writeErr: wantErr,
		}
		service := NewCatalogEmbeddingService(&fakeDB{}, products, &fakeEmbedder{},
			testSearchConfig())

		report, err := service.Backfill(context.Background(),
			domain.Tenant{AccountID: testAccountID}, false)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Backfill() = %v, want %v", err, wantErr)
		}
		if report.Embedded != 0 || report.Rounds != 0 {
			t.Errorf("report = %+v, want nothing counted", report)
		}
	})

	// The write succeeded and the transaction did not, so the rows the repository reported are
	// not in the database.
	t.Run("the round fails to commit", func(t *testing.T) {
		products := &fakeCatalogEmbedding{
			pages: [][]domain.ProductEmbeddingInput{productPage("Cemento Portland 50kg")},
		}
		service := NewCatalogEmbeddingService(&lateFailingDB{err: wantErr}, products,
			&fakeEmbedder{}, testSearchConfig())

		report, err := service.Backfill(context.Background(),
			domain.Tenant{AccountID: testAccountID}, false)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Backfill() = %v, want %v", err, wantErr)
		}
		if report.Embedded != 0 {
			t.Errorf("embedded = %d, want 0: the round never committed", report.Embedded)
		}
	})
}

// Pairing a product with another product's vector is the silent wrong match this layer exists
// to prevent, so a short answer stops the run.
func TestCatalogEmbeddingService_RefusesAMisalignedEmbeddingAnswer(t *testing.T) {
	products := &fakeCatalogEmbedding{
		pages: [][]domain.ProductEmbeddingInput{productPage("Cemento", "Cal")},
	}
	service := NewCatalogEmbeddingService(&fakeDB{}, products, &fakeEmbedder{short: true},
		testSearchConfig())

	_, err := service.Backfill(context.Background(), domain.Tenant{AccountID: testAccountID}, false)
	if err == nil {
		t.Fatal("Backfill() = nil error, want the short answer refused")
	}
	if len(products.written) != 0 {
		t.Errorf("wrote %d batches, want none", len(products.written))
	}
}
