package services

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"image"
	"image/png"
	"io"
	"testing"
)

type invoiceLogoStub struct {
	calls       int
	account, id uuid.UUID
	content     []byte
}

func (s *invoiceLogoStub) DownloadLogo(_ context.Context, account, id uuid.UUID) (*domain.StoredObject, error) {
	s.calls++
	s.account = account
	s.id = id
	return &domain.StoredObject{Body: io.NopCloser(bytes.NewReader(s.content))}, nil
}

func TestInvoiceLogo_UsesFrozenAssetWithinItsAccount(t *testing.T) {
	account, id := uuid.New(), uuid.New()
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 400, 80))); err != nil {
		t.Fatal(err)
	}
	loader := &invoiceLogoStub{content: imageBytes.Bytes()}
	s := &InvoiceService{logos: loader, settings: InvoiceSettings{LogoMaxBytes: 4096, LogoMaxPixels: 100000}}
	address := "http://localhost:8000/v1/public/account-logos/" + account.String() + "/" + id.String()
	logo := s.loadInvoiceLogo(context.Background(), account, &address)
	if logo == nil || logo.ContentType != "image/png" || loader.account != account || loader.id != id {
		t.Fatal("frozen account logo was not loaded")
	}
	s.settings.LogoMaxPixels = 100
	if s.loadInvoiceLogo(context.Background(), account, &address) != nil {
		t.Fatal("oversized image decoded")
	}
	loader.calls = 1
	s.settings.LogoMaxPixels = 100000
	other := "https://example.com/v1/public/account-logos/" + uuid.New().String() + "/" + id.String()
	if s.loadInvoiceLogo(context.Background(), account, &other) != nil || loader.calls != 1 {
		t.Fatal("another account's logo was read")
	}
	external := "https://example.com/image.png"
	if s.loadInvoiceLogo(context.Background(), account, &external) != nil || loader.calls != 1 {
		t.Fatal("external asset was fetched")
	}
}

func TestInvoiceLogo_InvalidStoredImageFallsBackToText(t *testing.T) {
	account := uuid.New()
	s := &InvoiceService{logos: &invoiceLogoStub{content: []byte("invalid")}, settings: InvoiceSettings{LogoMaxBytes: 4096, LogoMaxPixels: 100000}}
	address := "/v1/public/account-logos/" + account.String() + "/" + uuid.New().String()
	if s.loadInvoiceLogo(context.Background(), account, &address) != nil {
		t.Fatal("invalid image accepted")
	}
}
