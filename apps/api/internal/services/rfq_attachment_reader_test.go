package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// spreadsheetContentType is the modern workbook type, which is what an ".xlsx" key is stored
// under. The reader picks its parser off the key, so a case whose type and key disagree would
// read as testing the type when it is testing neither.
const spreadsheetContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func storedAttachment(kind domain.AttachmentType, key string) domain.ClaimedAttachment {
	return domain.ClaimedAttachment{ID: uuid.New(), AccountID: uuid.New(), BranchID: uuid.New(),
		RFQID: uuid.New(), Type: kind, StorageKey: key}
}

/*
 * The table below is the sweep's half of the same normalisation the inline intake does, and it is
 * deliberately the twin of TestRFQService_ReadFileContent_ShapesEachFormatForTheModel: a format
 * that reached the model one way through the request path and another way through the sweep would
 * be two engines wearing one name.
 */
func TestRFQAttachmentService_ReadStoredAttachment_ShapesEachStoredFormatForTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		kind        domain.AttachmentType
		key         string
		contentType string
		data        string
		wantKind    domain.ContentKind
		wantText    string
	}{
		{
			name:        "a photographed list reaches the model as a picture",
			kind:        domain.AttachmentTypeImage,
			key:         "accounts/a/rfqs/r/f.jpg",
			contentType: "image/jpeg",
			data:        "\xFF\xD8not-a-real-jpeg",
			wantKind:    domain.ContentKindImage,
			// An image yields no text of its own: the file is the record.
			wantText: "",
		},
		{
			name:        "a PDF reaches the model as a document",
			kind:        domain.AttachmentTypePDF,
			key:         "accounts/a/rfqs/r/f.pdf",
			contentType: "application/pdf",
			data:        "%PDF-1.4 not-a-real-pdf",
			wantKind:    domain.ContentKindDocument,
			wantText:    "",
		},
		{
			name:        "a text file reaches the model as its own words",
			kind:        domain.AttachmentTypeText,
			key:         "accounts/a/rfqs/r/f.txt",
			contentType: "text/plain",
			data:        "10 bolsas de cemento",
			wantKind:    domain.ContentKindText,
			wantText:    "10 bolsas de cemento",
		},
		{
			name:        "a spreadsheet is flattened to rows first",
			kind:        domain.AttachmentTypeSpreadsheet,
			key:         "accounts/a/rfqs/r/f.csv",
			contentType: "text/csv",
			data:        "Producto,Cantidad\nCemento,10\n",
			wantKind:    domain.ContentKindText,
			wantText:    "Producto\tCantidad\nCemento\t10",
		},
		{
			// Stored under .xls before the type mapped to .csv: the bytes still decide.
			name:        "a Windows csv stored as .xls reads as the csv it is",
			kind:        domain.AttachmentTypeSpreadsheet,
			key:         "accounts/a/rfqs/r/f.xls",
			contentType: "application/vnd.ms-excel",
			data:        "Producto;Cantidad\nCemento;10\n",
			wantKind:    domain.ContentKindText,
			wantText:    "Producto\tCantidad\nCemento\t10",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := &fakeObjectStorage{data: []byte(tc.data), contentType: tc.contentType}
			reader := storedReader(storage, testRFQConfig(), nil)

			block, text, err := reader.ReadStoredAttachment(context.Background(),
				storedAttachment(tc.kind, tc.key))
			if err != nil {
				t.Fatalf("ReadStoredAttachment() = %v", err)
			}
			if block.Kind != tc.wantKind {
				t.Errorf("block kind = %q, want %q", block.Kind, tc.wantKind)
			}
			if text != tc.wantText {
				t.Errorf("kept text = %q, want %q", text, tc.wantText)
			}
			// A picture and a document carry their bytes; text carries its words.
			if tc.wantKind == domain.ContentKindText {
				if block.Text != tc.wantText {
					t.Errorf("block text = %q, want %q", block.Text, tc.wantText)
				}
			} else if string(block.Data) != tc.data {
				t.Errorf("block carried %d bytes, want the file's %d", len(block.Data), len(tc.data))
			}
			if len(storage.keys) != 1 || storage.keys[0] != tc.key {
				t.Errorf("downloaded %v, want the attachment's own key %q", storage.keys, tc.key)
			}
		})
	}
}

// A recording has no layout a model can use, so it becomes text before it reaches one — and the
// transcriber is handed the stored object's own name and media type, not the request's.
func TestRFQAttachmentService_ReadStoredAttachment_TranscribesAStoredRecording(t *testing.T) {
	t.Parallel()
	transcriber := &fakeTranscriber{text: "mandame 10 bolsas de cemento"}
	storage := &fakeObjectStorage{data: []byte("RIFF-not-a-real-wav"), contentType: "audio/mpeg"}
	reader := storedReader(storage, testRFQConfig(), transcriber)

	block, text, err := reader.ReadStoredAttachment(context.Background(),
		storedAttachment(domain.AttachmentTypeAudio, "accounts/a/rfqs/r/nota.mp3"))
	if err != nil {
		t.Fatalf("ReadStoredAttachment() = %v", err)
	}
	if block.Kind != domain.ContentKindText || block.Text != transcriber.text {
		t.Errorf("block = %q/%q, want the transcript as text", block.Kind, block.Text)
	}
	if text != transcriber.text {
		t.Errorf("kept text = %q, want the transcript", text)
	}
	if transcriber.audio.Filename != "nota.mp3" || transcriber.audio.MediaType != "audio/mpeg" {
		t.Errorf("transcribed %q/%q, want the stored object's name and media type",
			transcriber.audio.Filename, transcriber.audio.MediaType)
	}
	if transcriber.scope.Operation != domain.AIOperationAudioTranscription {
		t.Errorf("operation = %q, want AUDIO_TRANSCRIPTION", transcriber.scope.Operation)
	}
}

/*
 * An unreadable file is refused here so the sweep can close it FAILED on its own, which is what
 * stops it being claimed forever — and every case below is a file whose bytes will not change, so
 * retrying it could only fail the same way. ErrNotConfigured is the exception and is not the
 * file's fault: an unbound transcriber is the deployment's problem and a later run finds it bound.
 */
func TestRFQAttachmentService_ReadStoredAttachment_RefusesWhatCannotBeReadAsAnOrder(t *testing.T) {
	t.Parallel()
	cfg := testRFQConfig()
	cases := []struct {
		name        string
		kind        domain.AttachmentType
		key         string
		contentType string
		data        string
		transcript  string
		transcriber bool
		wantErr     error
		// Several guards below answer with the same error class, so the class alone cannot say
		// which one refused the file. wantMessage is what tells them apart.
		wantMessage string
	}{
		{
			name: "a spreadsheet whose bytes are not a sheet", kind: domain.AttachmentTypeSpreadsheet,
			key: "accounts/a/rfqs/r/f.xlsx", contentType: spreadsheetContentType,
			data: "PK\x03\x04 is a zip signature on no archive", wantErr: domain.ErrInvalidInput,
			wantMessage: "could not be read",
		},
		{
			name: "a legacy .xls workbook", kind: domain.AttachmentTypeSpreadsheet,
			key: "accounts/a/rfqs/r/f.xls", contentType: "application/vnd.ms-excel",
			data: "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1workbook", wantErr: domain.ErrInvalidInput,
			wantMessage: "legacy .xls",
		},
		{
			name: "a spreadsheet with no rows in it", kind: domain.AttachmentTypeSpreadsheet,
			key: "accounts/a/rfqs/r/f.csv", contentType: "text/csv", data: "\n\n",
			wantErr: domain.ErrInvalidInput, wantMessage: "has no rows",
		},
		{
			name: "a sheet that is a catalog rather than an order",
			kind: domain.AttachmentTypeSpreadsheet, key: "accounts/a/rfqs/r/f.csv",
			contentType: "text/csv", data: strings.Repeat("a\n", cfg.MaxSpreadsheetRows+1),
			wantErr: domain.ErrInvalidInput, wantMessage: "catalog rather than an order",
		},
		{
			name: "a text file with nothing written in it", kind: domain.AttachmentTypeText,
			key: "accounts/a/rfqs/r/f.txt", contentType: "text/plain", data: "   \n  ",
			wantErr: domain.ErrInvalidInput, wantMessage: "cannot be blank",
		},
		{
			name: "a text file longer than the engine reads", kind: domain.AttachmentTypeText,
			key: "accounts/a/rfqs/r/f.txt", contentType: "text/plain",
			data: strings.Repeat("x", cfg.MaxTextCharacters+1), wantErr: domain.ErrInvalidInput,
			wantMessage: "cannot exceed",
		},
		{
			name: "a recording with nothing said in it", kind: domain.AttachmentTypeAudio,
			key: "accounts/a/rfqs/r/f.mp3", contentType: "audio/mpeg", data: "RIFF",
			transcriber: true, transcript: "  ", wantErr: domain.ErrInvalidInput,
			wantMessage: "cannot be blank",
		},
		{
			name: "a recording with no transcriber bound", kind: domain.AttachmentTypeAudio,
			key: "accounts/a/rfqs/r/f.mp3", contentType: "audio/mpeg", data: "RIFF",
			wantErr: domain.ErrNotConfigured,
		},
		{
			name: "an attachment whose file was never stored", kind: domain.AttachmentTypePDF,
			key: "", contentType: "application/pdf", data: "%PDF", wantErr: domain.ErrInvalidInput,
			wantMessage: "has no stored file",
		},
		{
			name: "a kind the engine cannot read as an order", kind: domain.AttachmentType("VIDEO"),
			key: "accounts/a/rfqs/r/f.mov", contentType: "video/quicktime", data: "moov",
			wantErr:     domain.ErrInvalidInput,
			wantMessage: "cannot be read as an order",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var transcriber domain.Transcriber
			if tc.transcriber {
				transcriber = &fakeTranscriber{text: tc.transcript}
			}
			reader := storedReader(
				&fakeObjectStorage{data: []byte(tc.data), contentType: tc.contentType}, cfg,
				transcriber)

			_, text, err := reader.ReadStoredAttachment(context.Background(),
				storedAttachment(tc.kind, tc.key))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMessage != "" && !strings.Contains(err.Error(), tc.wantMessage) {
				t.Errorf("err = %v, want the refusal to name %q", err, tc.wantMessage)
			}
			if text != "" {
				t.Errorf("kept text = %q, want nothing recorded as read", text)
			}
		})
	}
}

// Storage being unreachable says nothing about the file, so the error travels as it is rather
// than being reshaped into one about the attachment.
func TestRFQAttachmentService_ReadStoredAttachment_ReportsAStorageFailureAsItIs(t *testing.T) {
	t.Parallel()
	down := errors.New("bucket unreachable")
	reader := storedReader(&fakeObjectStorage{err: down}, testRFQConfig(), nil)

	if _, _, err := reader.ReadStoredAttachment(context.Background(),
		storedAttachment(domain.AttachmentTypePDF, "accounts/a/rfqs/r/f.pdf")); !errors.Is(err, down) {
		t.Fatalf("err = %v, want the storage failure", err)
	}
}
