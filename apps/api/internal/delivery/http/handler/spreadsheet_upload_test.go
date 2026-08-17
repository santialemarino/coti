package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOpenSpreadsheetUpload_ReturnsTheConfiguredFile(t *testing.T) {
	t.Parallel()
	context, recorder := spreadsheetUploadTestContext(t, "precios.csv", "codigo,precio\nCEM-001,10000")

	file, filename, ok := openSpreadsheetUpload(context, 1024)
	if !ok {
		t.Fatalf("openSpreadsheetUpload() status = %d, want a file", recorder.Code)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if filename != "precios.csv" || !strings.Contains(string(content), "CEM-001") {
		t.Fatalf("filename, content = %q, %q; want the uploaded spreadsheet", filename, content)
	}
}

func TestOpenSpreadsheetUpload_RejectsTheConfiguredLimit(t *testing.T) {
	t.Parallel()
	context, recorder := spreadsheetUploadTestContext(t, "catalogo.csv", strings.Repeat("x", 128))

	file, _, ok := openSpreadsheetUpload(context, 64)
	if ok || file != nil {
		t.Fatal("openSpreadsheetUpload() accepted a request above the configured limit")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), "file too large") {
		t.Fatalf("response = %d %q, want a clear 413", recorder.Code, recorder.Body.String())
	}
}

func spreadsheetUploadTestContext(t *testing.T, filename, content string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/import/preview", bytes.NewReader(body.Bytes()))
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return context, recorder
}
