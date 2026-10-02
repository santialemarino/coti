package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type accountLogoStorage struct {
	key         string
	contentType string
	data        []byte
}

func (s *accountLogoStorage) Upload(_ context.Context, key, contentType string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	s.key, s.contentType, s.data = key, contentType, data
	return nil
}

func (s *accountLogoStorage) Download(_ context.Context, key string) (*domain.StoredObject, error) {
	if key != s.key {
		return nil, fmt.Errorf("%w: %s", domain.ErrNotFound, key)
	}
	return &domain.StoredObject{
		Body:        io.NopCloser(bytes.NewReader(s.data)),
		ContentType: s.contentType,
		Size:        int64(len(s.data)),
	}, nil
}

func (*accountLogoStorage) GenerateSignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func TestAccountService_UploadLogo_StoresDetectedImageUnderAccount(t *testing.T) {
	t.Parallel()
	storage := &accountLogoStorage{}
	service := (&AccountService{}).WithLogoStorage(storage, 1024)
	accountID := uuid.New()
	data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)

	logo, err := service.UploadLogo(context.Background(), domain.Tenant{AccountID: accountID},
		domain.AccountLogoUpload{ContentType: "image/png", Size: int64(len(data)), Content: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("UploadLogo() error = %v", err)
	}
	if logo.AccountID != accountID || logo.ID == uuid.Nil {
		t.Fatalf("UploadLogo() = %#v, want account %s and a generated id", logo, accountID)
	}
	wantPrefix := "accounts/" + accountID.String() + "/brand/"
	if !strings.HasPrefix(storage.key, wantPrefix) {
		t.Errorf("stored key = %q, want prefix %q", storage.key, wantPrefix)
	}
	if storage.contentType != "image/png" || !bytes.Equal(storage.data, data) {
		t.Errorf("stored image = (%q, %v), want image/png and original bytes", storage.contentType, storage.data)
	}

	stored, err := service.DownloadLogo(context.Background(), accountID, logo.ID)
	if err != nil {
		t.Fatalf("DownloadLogo() error = %v", err)
	}
	defer stored.Body.Close()
	got, err := io.ReadAll(stored.Body)
	if err != nil {
		t.Fatalf("read downloaded logo: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("downloaded logo = %v, want %v", got, data)
	}
}

func TestAccountService_UploadLogo_RefusesInvalidFiles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file domain.AccountLogoUpload
	}{
		{"empty", domain.AccountLogoUpload{ContentType: "image/png", Content: bytes.NewReader(nil)}},
		{"too large", domain.AccountLogoUpload{ContentType: "image/png", Size: 1025, Content: bytes.NewReader(nil)}},
		{"unsupported type", domain.AccountLogoUpload{ContentType: "image/svg+xml", Size: 4, Content: strings.NewReader("<svg")}},
		{"spoofed type", domain.AccountLogoUpload{ContentType: "image/png", Size: 5, Content: strings.NewReader("hello")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := &accountLogoStorage{}
			service := (&AccountService{}).WithLogoStorage(storage, 1024)
			if _, err := service.UploadLogo(context.Background(), domain.Tenant{AccountID: uuid.New()}, tc.file); err == nil {
				t.Fatal("UploadLogo() error = nil, want refusal")
			}
			if storage.key != "" {
				t.Errorf("refused upload stored %q", storage.key)
			}
		})
	}
}
