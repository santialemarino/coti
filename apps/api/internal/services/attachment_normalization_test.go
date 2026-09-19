package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// fakeObjectStorage serves one stored file, which is all a stored-file reader ever asks it for.
type fakeObjectStorage struct {
	data        []byte
	contentType string
	err         error
	keys        []string
}

func (f *fakeObjectStorage) Upload(context.Context, string, string, io.Reader) error { return nil }

func (f *fakeObjectStorage) Download(_ context.Context, key string) (*domain.StoredObject, error) {
	f.keys = append(f.keys, key)
	if f.err != nil {
		return nil, f.err
	}
	return &domain.StoredObject{
		Body:        io.NopCloser(bytes.NewReader(f.data)),
		ContentType: f.contentType,
		Size:        int64(len(f.data)),
	}, nil
}

func (f *fakeObjectStorage) GenerateSignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

// storedReader builds the attachment service as the sweep wires it, over one stored file.
func storedReader(storage domain.ObjectStorage, cfg config.RFQConfig,
	transcriber domain.Transcriber) *RFQAttachmentService {
	return NewRFQAttachmentService(nil, nil, storage, config.StorageConfig{}, nil).
		WithStoredReading(transcriber, cfg.MaxTextCharacters, cfg.MaxSpreadsheetRows)
}

/*
 * A sheet's own size decides whether it is an order or a catalog, never which door it arrived
 * through. Holding the two readers to different row limits means one file becomes a draft or a
 * permanent retry loop depending on how it reached the engine: a sheet the sweep accepts and the
 * extractor then refuses leaves its attachments claimed, to be re-read every reclaim window.
 */
func TestSpreadsheetOrderText_BothDoorsRefuseTheSameCatalog(t *testing.T) {
	t.Parallel()
	cfg := testRFQConfig()
	// One cell per row, so the sheet is past the row limit while staying well under the
	// character limit. Without that the character limit would be what refuses it and this
	// would pass with the row limit gone.
	sheet := []byte(strings.Repeat("a\n", cfg.MaxSpreadsheetRows+1))
	if len(sheet) > cfg.MaxTextCharacters {
		t.Fatalf("fixture is %d characters and the cap is %d: the row limit is not what is "+
			"under test", len(sheet), cfg.MaxTextCharacters)
	}

	inline := &RFQService{cfg: cfg}
	if _, err := inline.readSpreadsheet("catalogo.csv", sheet); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("inline intake err = %v, want ErrInvalidInput", err)
	}

	sweep := storedReader(&fakeObjectStorage{data: sheet, contentType: "text/csv"}, cfg, nil)
	_, _, err := sweep.ReadStoredAttachment(context.Background(), domain.ClaimedAttachment{
		Type: domain.AttachmentTypeSpreadsheet, StorageKey: "accounts/a/rfqs/r/f.csv"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sweep err = %v, want ErrInvalidInput", err)
	}
}
