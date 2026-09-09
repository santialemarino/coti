//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/pdf"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

type failingRepresentationStorage struct{}

func (failingRepresentationStorage) Upload(context.Context, string, string, io.Reader) error {
	return errors.New("storage unavailable")
}

func (failingRepresentationStorage) Download(context.Context, string) (*domain.StoredObject, error) {
	return nil, errors.New("storage unavailable")
}

func (failingRepresentationStorage) GenerateSignedURL(context.Context, string,
	time.Duration) (string, error) {
	return "", errors.New("storage unavailable")
}

func TestQuoteRepresentation_GenerateReplayAndPublicDelivery(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Representation", false)
	ctx := context.Background()
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM quote_representation WHERE account_id = $1`, seed.tenant.AccountID)
	})
	baseURL, _ := url.Parse("https://files.test")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL, storage.NewURLSigner([]byte("representation-test-secret-at-least-32-chars"), nil))
	if err != nil {
		t.Fatal(err)
	}
	service := services.NewQuoteRepresentationService(e.db, repository.NewQuoteRepresentationRepository(), repository.NewQuoteRepository(), objects, nil, pdf.NewQuoteRenderer(), 15*time.Minute, nil, nil)
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO quote_item_alternative
	  (account_id, quote_item_id, product_id, type, origin, price_snapshot,
	   approved_by_seller, rank, confidence_score)
	  VALUES ($1, $2, $3, 'PRODUCT', 'SELLER', 48.00, TRUE, 1, 0.9000),
	         ($1, $2, $3, 'PRODUCT', 'SELLER', 47.00, FALSE, 2, 0.8000)`,
		seed.tenant.AccountID, seed.draft.Items[0].ID, seed.draft.Items[0].ProductID); err != nil {
		t.Fatal(err)
	}
	first, err := service.Ensure(ctx, seed.tenant, seed.draft.Quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replay || first.Representation.Payload.Total != "100.00" ||
		len(first.Representation.Payload.Items[0].Alternatives) != 1 ||
		first.Representation.Payload.Items[0].Alternatives[0].UnitPrice != "48.00" {
		t.Fatalf("unexpected bundle: %+v", first)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE product SET canonical_name = 'Renamed' WHERE id = $1`, seed.draft.Items[0].ProductID); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Ensure(ctx, seed.tenant, seed.draft.Quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.Representation.ID != first.Representation.ID || replay.Representation.Payload.Items[0].ProductName != first.Representation.Payload.Items[0].ProductName {
		t.Fatal("replay changed the immutable snapshot")
	}
	wrong := seed.tenant
	wrong.BranchID = uuid.New()
	if _, err := service.Ensure(ctx, wrong, seed.draft.Quote.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("wrong branch: %v", err)
	}
	var evaluations int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*)
	  FROM quote_quality_evaluation evaluation
	  JOIN quote_ai_generation generation ON generation.id = evaluation.generation_id
	  WHERE generation.quote_id = $1`, seed.draft.Quote.ID).Scan(&evaluations); err != nil {
		t.Fatal(err)
	}
	if evaluations != 0 {
		t.Fatalf("evaluations before delivery = %d, want none", evaluations)
	}
	sender := &captureWhatsAppSender{}
	delivery := e.quoteDeliveryService(t, sender, &stagedQuoteEmailSender{}, nil).WithRepresentationService(service)
	sent, err := delivery.Send(ctx, seed.tenant, seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550101"})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(sent.Deliveries[0].PublicURL, "https://quotes.test/quotes/")
	public, err := delivery.ResolvePublic(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if public.Payload == nil || public.PDFURL == nil || public.Message == nil || strings.Contains(*public.Message, domain.QuotePublicURLPlaceholder) {
		t.Fatalf("incomplete public response: %+v", public)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_send SET expires_at = now() - interval '1 second' WHERE version_id = $1`, seed.draft.Version.ID); err != nil {
		t.Fatal(err)
	}
	public, err = delivery.ResolvePublic(ctx, token)
	if err != nil || public.Status != "EXPIRED" || public.Payload != nil || public.PDFURL != nil {
		t.Fatalf("expired response: %+v, %v", public, err)
	}
}

func TestQuoteRepresentation_StorageFailureLeavesNoUsableReference(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Representation storage failure", false)
	service := services.NewQuoteRepresentationService(e.db,
		repository.NewQuoteRepresentationRepository(), repository.NewQuoteRepository(),
		failingRepresentationStorage{}, nil, pdf.NewQuoteRenderer(), 15*time.Minute, nil, nil)
	_, err := service.Ensure(context.Background(), seed.tenant, seed.draft.Quote.ID)
	if !errors.Is(err, domain.ErrRepresentationUnavailable) {
		t.Fatalf("Ensure() = %v, want ErrRepresentationUnavailable", err)
	}
	var representations int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM quote_representation WHERE quote_id = $1`,
		seed.draft.Quote.ID).Scan(&representations); err != nil {
		t.Fatal(err)
	}
	if representations != 0 {
		t.Fatalf("representation rows = %d, want none", representations)
	}
	baseURL, _ := url.Parse("https://files.test")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL,
		storage.NewURLSigner([]byte("representation-retry-secret-32chars"), nil))
	if err != nil {
		t.Fatal(err)
	}
	retry := services.NewQuoteRepresentationService(e.db,
		repository.NewQuoteRepresentationRepository(), repository.NewQuoteRepository(),
		objects, nil, pdf.NewQuoteRenderer(), 15*time.Minute, nil, nil)
	if _, err := retry.Ensure(context.Background(), seed.tenant, seed.draft.Quote.ID); err != nil {
		t.Fatalf("retry Ensure() = %v, want success", err)
	}
}
