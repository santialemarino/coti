package handler

import (
	"encoding/json"
	"testing"
	"time"

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
