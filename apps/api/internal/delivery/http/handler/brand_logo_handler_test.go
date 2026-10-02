package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type brandLogoServiceStub struct {
	accountID uuid.UUID
	logoID    uuid.UUID
}

func (s brandLogoServiceStub) DownloadLogo(
	_ context.Context, accountID, logoID uuid.UUID,
) (*domain.StoredObject, error) {
	if accountID != s.accountID || logoID != s.logoID {
		return nil, fmt.Errorf("%w: account logo", domain.ErrNotFound)
	}
	return &domain.StoredObject{
		Body:        io.NopCloser(strings.NewReader("png")),
		ContentType: "image/png",
		Size:        3,
	}, nil
}

func (brandLogoServiceStub) UploadLogo(
	context.Context, domain.Tenant, domain.AccountLogoUpload,
) (*domain.AccountLogo, error) {
	return nil, nil
}

func TestBrandLogoHandler_Get_ServesTheImageInlineAndImmutable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := brandLogoServiceStub{accountID: uuid.New(), logoID: uuid.New()}
	router := gin.New()
	router.GET("/v1/public/account-logos/:accountId/:logoId", NewBrandLogoHandler(service, 1024).Get)
	recorder := httptest.NewRecorder()
	target := fmt.Sprintf("/v1/public/account-logos/%s/%s", service.accountID, service.logoID)

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	if recorder.Code != http.StatusOK || recorder.Body.String() != "png" {
		t.Fatalf("response = (%d, %q), want (200, png)", recorder.Code, recorder.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":           "image/png",
		"Content-Disposition":    "inline",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "public, max-age=31536000, immutable",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}
