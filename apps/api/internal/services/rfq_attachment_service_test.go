package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func attachmentService(maxFileSize int64) *RFQAttachmentService {
	return &RFQAttachmentService{cfg: config.StorageConfig{MaxFileSize: maxFileSize}}
}

func TestRFQAttachmentService_AcceptedFormat_MapsEachAcceptedTypeToItsStoredKind(t *testing.T) {
	t.Parallel()
	service := attachmentService(1024)

	cases := []struct {
		contentType string
		wantType    domain.AttachmentType
		wantExt     string
	}{
		{"application/pdf", domain.AttachmentTypePDF, "pdf"},
		{"image/jpeg", domain.AttachmentTypeImage, "jpg"},
		{"image/png", domain.AttachmentTypeImage, "png"},
		{"text/csv", domain.AttachmentTypeSpreadsheet, "csv"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			domain.AttachmentTypeSpreadsheet, "xlsx"},
		{"audio/mpeg", domain.AttachmentTypeAudio, "mp3"},
		{"text/plain", domain.AttachmentTypeText, "txt"},
	}
	for _, tc := range cases {
		t.Run(tc.contentType, func(t *testing.T) {
			t.Parallel()
			format, err := service.acceptedFormat(domain.AttachmentUpload{ContentType: tc.contentType, Size: 10})
			if err != nil {
				t.Fatalf("acceptedFormat(%q) = %v, want no error", tc.contentType, err)
			}
			if format.Type != tc.wantType {
				t.Errorf("type = %q, want %q", format.Type, tc.wantType)
			}
			if format.Extension != tc.wantExt {
				t.Errorf("extension = %q, want %q", format.Extension, tc.wantExt)
			}
		})
	}
}

func TestRFQAttachmentService_AcceptedFormat_RefusesATypeOutsideTheSet(t *testing.T) {
	t.Parallel()
	service := attachmentService(1024)

	// An HTML upload is the one worth naming: it is inert on the way out only because the file
	// route sends it as an attachment with nosniff, and it has no business being stored at all.
	for _, contentType := range []string{
		"text/html", "application/javascript", "image/svg+xml", "application/octet-stream", "",
	} {
		_, err := service.acceptedFormat(domain.AttachmentUpload{ContentType: contentType, Size: 10})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("acceptedFormat(%q) = %v, want ErrInvalidInput", contentType, err)
		}
		if got := domain.CodeOf(err); got != domain.CodeUnsupportedFileType {
			t.Errorf("acceptedFormat(%q) code = %q, want UNSUPPORTED_FILE_TYPE", contentType, got)
		}
	}
}

func TestRFQAttachmentService_AcceptedFormat_RefusesBySize(t *testing.T) {
	t.Parallel()
	service := attachmentService(100)

	cases := []struct {
		name string
		size int64
	}{
		{"over the limit", 101},
		{"empty", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := service.acceptedFormat(domain.AttachmentUpload{ContentType: "application/pdf", Size: tc.size})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("acceptedFormat(size=%d) = %v, want ErrInvalidInput", tc.size, err)
			}
		})
	}

	// The limit itself is accepted: it is a maximum, not a value to stay under.
	if _, err := service.acceptedFormat(domain.AttachmentUpload{ContentType: "application/pdf", Size: 100}); err != nil {
		t.Errorf("acceptedFormat(size=100) = %v, want the limit itself to be accepted", err)
	}
}

func TestRFQAttachmentService_AcceptedFormat_ReportsSizeAndTypeWithDifferentCodes(t *testing.T) {
	t.Parallel()
	service := attachmentService(100)

	_, tooBig := service.acceptedFormat(domain.AttachmentUpload{ContentType: "application/pdf", Size: 101})
	if got := domain.CodeOf(tooBig); got != domain.CodeFileTooLarge {
		t.Errorf("oversized code = %q, want FILE_TOO_LARGE", got)
	}
	// Size is checked first, so a file that is both too big and the wrong type reports the size:
	// telling someone their unsupported file is also too large helps nobody.
	_, wrongType := service.acceptedFormat(domain.AttachmentUpload{ContentType: "text/html", Size: 10})
	if got := domain.CodeOf(wrongType); got != domain.CodeUnsupportedFileType {
		t.Errorf("unsupported code = %q, want UNSUPPORTED_FILE_TYPE", got)
	}
}

// AC4 lives in this function: the account leads the key, so isolation is visible in the path.
func TestAttachmentKey_LeadsWithTheAccount(t *testing.T) {
	t.Parallel()
	accountID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	rfqID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	attachmentID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	key := attachmentKey(accountID, rfqID, attachmentID, "pdf")

	want := "accounts/11111111-1111-1111-1111-111111111111/rfqs/" +
		"22222222-2222-2222-2222-222222222222/33333333-3333-3333-3333-333333333333.pdf"
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
	if !strings.HasPrefix(key, "accounts/"+accountID.String()+"/") {
		t.Errorf("key = %q, want the account at the front", key)
	}
}

// The extension comes from the accepted format, never from the client's filename, so a name
// like "factura.pdf.html" cannot decide what the stored object is called.
func TestAttachmentKey_UsesTheExtensionTheFormatDecided(t *testing.T) {
	t.Parallel()
	id := uuid.New()

	if got := attachmentKey(id, id, id, "xlsx"); !strings.HasSuffix(got, ".xlsx") {
		t.Errorf("key = %q, want it to end in .xlsx", got)
	}
}

func TestNormalizeContentType_DropsTheParametersABrowserAppends(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"text/csv; charset=utf-8": "text/csv",
		"application/pdf":         "application/pdf",
		"TEXT/CSV":                "text/csv",
		"  text/plain  ":          "text/plain",
	}
	for raw, want := range cases {
		if got := normalizeContentType(raw); got != want {
			t.Errorf("normalizeContentType(%q) = %q, want %q", raw, got, want)
		}
	}
}
