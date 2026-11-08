package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type dataExportServiceStub struct {
	request domain.DataExportRequest
	file    *domain.DataExportFile
	err     error
}

func (s *dataExportServiceStub) Export(
	_ context.Context, _ domain.Tenant, request domain.DataExportRequest,
) (*domain.DataExportFile, error) {
	s.request = request
	return s.file, s.err
}

func serveDataExport(service *dataExportServiceStub, target string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	c.Params = gin.Params{{Key: "entity", Value: "quotes"}}
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin})
	NewDataExportHandler(service).Export(c)
	return recorder
}

func TestDataExportHandlerDownloadsTheFile(t *testing.T) {
	branchID := uuid.NewString()
	service := &dataExportServiceStub{file: &domain.DataExportFile{
		Filename: "cotizaciones-2026-11-05.csv", ContentType: "text/csv; charset=utf-8",
		Content: []byte("numero\r\n1\r\n"),
	}}
	recorder := serveDataExport(service,
		"/v1/exports/quotes?format=csv&branch_id="+branchID+"&from=2026-11-01&to=2026-11-05")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	want := domain.DataExportRequest{Entity: domain.DataExportQuotes, Format: domain.DataExportCSV,
		BranchID: branchID, From: "2026-11-01", To: "2026-11-05"}
	if service.request != want {
		t.Fatalf("request = %+v, want %+v", service.request, want)
	}
	for header, value := range map[string]string{
		"Content-Type":        "text/csv; charset=utf-8",
		"Content-Disposition": `attachment; filename="cotizaciones-2026-11-05.csv"`,
		"Cache-Control":       "private, no-store",
	} {
		if got := recorder.Header().Get(header); got != value {
			t.Fatalf("%s = %q, want %q", header, got, value)
		}
	}
	if recorder.Body.String() != "numero\r\n1\r\n" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestDataExportHandlerAnswersAnInvalidRequestWith422(t *testing.T) {
	service := &dataExportServiceStub{err: fmt.Errorf("%w: format must be csv or xml", domain.ErrInvalidInput)}
	recorder := serveDataExport(service, "/v1/exports/quotes?format=json")
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	if recorder.Header().Get("Content-Disposition") != "" {
		t.Fatal("an error response must not be offered as a download")
	}
}
