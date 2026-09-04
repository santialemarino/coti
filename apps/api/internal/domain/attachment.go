package domain

import (
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
)

// AttachmentType is the kind of file an RFQ arrived with. The multi-format engine reads each
// kind differently, which is why the kind is stored rather than derived on the way out.
type AttachmentType string

const (
	AttachmentTypeImage       AttachmentType = "IMAGE"
	AttachmentTypePDF         AttachmentType = "PDF"
	AttachmentTypeSpreadsheet AttachmentType = "SPREADSHEET"
	AttachmentTypeAudio       AttachmentType = "AUDIO"
	AttachmentTypeText        AttachmentType = "TEXT"
)

// AttachmentProcessingStatus is how far the multi-format engine has got with one attachment.
// Storing a file leaves it PENDING; nothing in the storage layer advances it.
type AttachmentProcessingStatus string

const (
	AttachmentProcessingPending    AttachmentProcessingStatus = "PENDING"
	AttachmentProcessingProcessing AttachmentProcessingStatus = "PROCESSING"
	AttachmentProcessingDone       AttachmentProcessingStatus = "DONE"
	AttachmentProcessingFailed     AttachmentProcessingStatus = "FAILED"
)

// AttachmentFormat is one accepted upload format: the kind it is stored as and the extension
// the object key ends in. The extension comes from here and never from the client's filename.
type AttachmentFormat struct {
	Type      AttachmentType
	Extension string
}

// attachmentFormats is the closed set of formats an RFQ attachment may be. A content type
// outside it is refused before any byte is stored.
var attachmentFormats = map[string]AttachmentFormat{
	"image/jpeg":      {AttachmentTypeImage, "jpg"},
	"image/png":       {AttachmentTypeImage, "png"},
	"image/webp":      {AttachmentTypeImage, "webp"},
	"image/heic":      {AttachmentTypeImage, "heic"},
	"application/pdf": {AttachmentTypePDF, "pdf"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {AttachmentTypeSpreadsheet, "xlsx"},
	"application/vnd.ms-excel": {AttachmentTypeSpreadsheet, "xls"},
	"text/csv":                 {AttachmentTypeSpreadsheet, "csv"},
	"audio/mpeg":               {AttachmentTypeAudio, "mp3"},
	"audio/mp4":                {AttachmentTypeAudio, "m4a"},
	"audio/ogg":                {AttachmentTypeAudio, "ogg"},
	"audio/wav":                {AttachmentTypeAudio, "wav"},
	"audio/webm":               {AttachmentTypeAudio, "webm"},
	"text/plain":               {AttachmentTypeText, "txt"},
}

// AttachmentFormatFor resolves a content type to the format it is stored as, reporting false
// for anything outside the accepted set. The parameters a browser appends — "; charset=utf-8"
// — are the caller's to strip.
func AttachmentFormatFor(contentType string) (AttachmentFormat, bool) {
	format, ok := attachmentFormats[contentType]
	return format, ok
}

// AcceptedAttachmentContentTypes lists every accepted content type, for an error a client can
// act on rather than a bare refusal. Sorted, because map order would reword the same refusal on
// every request.
func AcceptedAttachmentContentTypes() []string {
	types := make([]string, 0, len(attachmentFormats))
	for contentType := range attachmentFormats {
		types = append(types, contentType)
	}
	slices.Sort(types)
	return types
}

// RFQAttachment is one file an RFQ arrived with. The bytes live in object storage; this row
// holds the reference and what the engine has made of it.
type RFQAttachment struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	RFQID     uuid.UUID
	Type      AttachmentType
	// StorageKey is the object key, held in the file_url column. A key rather than a URL
	// because every link this layer hands out expires, and the reference must not.
	StorageKey       *string
	ExtractedText    *string
	ProcessingStatus AttachmentProcessingStatus
	CreatedAt        time.Time
	ProcessedAt      *time.Time
}

// NewRFQAttachment is the input for recording a stored file against an RFQ.
type NewRFQAttachment struct {
	ID         uuid.UUID
	RFQID      uuid.UUID
	Type       AttachmentType
	StorageKey string
}

// AttachmentUpload is one file offered for an RFQ. Size is what the transport reported, so it
// is checked before a single byte reaches storage.
type AttachmentUpload struct {
	ContentType string
	Size        int64
	Content     io.Reader
}

// AttachmentLink is one attachment paired with a link that serves it until the link expires.
type AttachmentLink struct {
	Attachment RFQAttachment
	URL        string
	ExpiresAt  time.Time
}
