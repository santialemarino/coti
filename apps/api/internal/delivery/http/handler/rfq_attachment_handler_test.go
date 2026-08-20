package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// An RFQ with no attachments has to serialize as [], not null: a client mapping over the field
// crashes on null, and "no attachments yet" is the ordinary case.
func TestToAttachmentResponses_EmptyIsAnArrayNotNull(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(toAttachmentResponses(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("encoded = %s, want []", encoded)
	}
}

func TestToAttachmentResponse_CarriesTheLinkAndItsDeadline(t *testing.T) {
	t.Parallel()
	expiresAt := time.Unix(1_700_000_900, 0).UTC()
	attachment := domain.RFQAttachment{
		ID:               uuid.New(),
		RFQID:            uuid.New(),
		Type:             domain.AttachmentTypePDF,
		ProcessingStatus: domain.AttachmentProcessingPending,
		CreatedAt:        time.Unix(1_700_000_000, 0).UTC(),
	}

	got := toAttachmentResponse(domain.AttachmentLink{
		Attachment: attachment, URL: "https://api.test/v1/files/k?sig=x", ExpiresAt: expiresAt,
	})

	if got.ID != attachment.ID || got.RFQID != attachment.RFQID {
		t.Errorf("ids = %v/%v, want %v/%v", got.ID, got.RFQID, attachment.ID, attachment.RFQID)
	}
	if got.Type != "PDF" || got.ProcessingStatus != "PENDING" {
		t.Errorf("type/status = %q/%q, want PDF/PENDING", got.Type, got.ProcessingStatus)
	}
	if got.URL != "https://api.test/v1/files/k?sig=x" {
		t.Errorf("url = %q, want the signed link", got.URL)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expires_at = %s, want %s", got.ExpiresAt, expiresAt)
	}
}

// The transport refuses an oversized body with 413 and the service refuses an oversized file
// with the same status: one refusal, one status, whichever layer caught it.
func TestRespond_TooLargeAnswersTheSameStatusAsTheTransport(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	Respond(c, fmt.Errorf("%w: the file is 200 bytes and the limit is 100", domain.ErrTooLarge))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if body.Code != string(domain.CodeFileTooLarge) {
		t.Errorf("code = %q, want %q", body.Code, domain.CodeFileTooLarge)
	}
}
