package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// fakeTranscriber stands in for the recording-to-text provider.
type fakeTranscriber struct {
	text  string
	err   error
	calls int
	audio domain.Audio
}

func (f *fakeTranscriber) Transcribe(_ context.Context, audio domain.Audio) (string, error) {
	f.calls++
	f.audio = audio
	return f.text, f.err
}

// fakeAttachmentStore records what the intake kept against the RFQ.
type fakeAttachmentStore struct {
	calls     int
	err       error
	data      []byte
	extracted string
	file      domain.AttachmentUpload
}

func (f *fakeAttachmentStore) StoreForRFQ(_ context.Context, _ domain.Tenant, _ uuid.UUID,
	file domain.AttachmentUpload, data []byte, extractedText string) error {
	f.calls++
	f.file = file
	f.data = data
	f.extracted = extractedText
	return f.err
}

func TestRFQService_ReadFileContent_ShapesEachFormatForTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		contentType   string
		filename      string
		data          string
		wantKind      domain.ContentKind
		wantExtracted string
	}{
		{
			name:        "a photographed list reaches the model as a picture",
			contentType: "image/jpeg",
			filename:    "pedido.jpg",
			data:        "\xFF\xD8not-a-real-jpeg",
			wantKind:    domain.ContentKindImage,
			// An image yields no text of its own: the file is the record.
			wantExtracted: "",
		},
		{
			name:          "a PDF reaches the model as a document",
			contentType:   "application/pdf",
			filename:      "pedido.pdf",
			data:          "%PDF-1.4 not-a-real-pdf",
			wantKind:      domain.ContentKindDocument,
			wantExtracted: "",
		},
		{
			name:          "a text file reaches the model as its own words",
			contentType:   "text/plain",
			filename:      "pedido.txt",
			data:          "10 bolsas de cemento",
			wantKind:      domain.ContentKindText,
			wantExtracted: "10 bolsas de cemento",
		},
		{
			name:          "a spreadsheet is flattened to rows first",
			contentType:   "text/csv",
			filename:      "pedido.csv",
			data:          "Producto,Cantidad\nCemento,10\n",
			wantKind:      domain.ContentKindText,
			wantExtracted: "Producto\tCantidad\nCemento\t10",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newRFQHarness(nil)
			format, ok := domain.AttachmentFormatFor(tc.contentType)
			if !ok {
				t.Fatalf("%q is not an accepted format", tc.contentType)
			}

			blocks, extracted, err := h.service.readFileContent(context.Background(),
				domain.FileRFQDraftInput{
					Filename: tc.filename,
					File:     domain.AttachmentUpload{ContentType: tc.contentType},
				}, format, []byte(tc.data))
			if err != nil {
				t.Fatalf("readFileContent returned %v", err)
			}
			if len(blocks) != 1 {
				t.Fatalf("produced %d blocks, want 1", len(blocks))
			}
			if blocks[0].Kind != tc.wantKind {
				t.Errorf("block kind = %q, want %q", blocks[0].Kind, tc.wantKind)
			}
			if extracted != tc.wantExtracted {
				t.Errorf("extracted text = %q, want %q", extracted, tc.wantExtracted)
			}
			// A picture and a document carry their bytes; text carries its words.
			if tc.wantKind == domain.ContentKindText {
				if blocks[0].Text != tc.wantExtracted {
					t.Errorf("block text = %q, want %q", blocks[0].Text, tc.wantExtracted)
				}
			} else if string(blocks[0].Data) != tc.data {
				t.Errorf("block carried %d bytes, want the file's %d",
					len(blocks[0].Data), len(tc.data))
			}
		})
	}
}

func TestRFQService_ReadFileContent_TranscribesARecordingBeforeReadingIt(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	transcriber := &fakeTranscriber{text: "necesito 30 bolsas de cemento"}
	h.service.WithFileIntake(&fakeAttachmentStore{}, transcriber, 10<<20)

	format, _ := domain.AttachmentFormatFor("audio/mp4")
	blocks, extracted, err := h.service.readFileContent(context.Background(),
		domain.FileRFQDraftInput{
			Filename: "nota.m4a",
			File:     domain.AttachmentUpload{ContentType: "audio/mp4"},
		}, format, []byte("fake-audio-bytes"))
	if err != nil {
		t.Fatalf("readFileContent returned %v", err)
	}
	if transcriber.calls != 1 {
		t.Fatalf("transcriber ran %d times, want 1", transcriber.calls)
	}
	if transcriber.audio.Filename != "nota.m4a" {
		t.Errorf("transcriber got filename %q, want the extension it decodes by",
			transcriber.audio.Filename)
	}
	if extracted != "necesito 30 bolsas de cemento" {
		t.Errorf("extracted %q, want the transcript", extracted)
	}
	if len(blocks) != 1 || blocks[0].Kind != domain.ContentKindText {
		t.Errorf("blocks = %+v, want the transcript as text", blocks)
	}
}

func TestRFQService_ReadFileContent_RefusesARecordingWithNoTranscriber(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)

	format, _ := domain.AttachmentFormatFor("audio/mp4")
	_, _, err := h.service.readFileContent(context.Background(),
		domain.FileRFQDraftInput{
			Filename: "nota.m4a",
			File:     domain.AttachmentUpload{ContentType: "audio/mp4"},
		}, format, []byte("fake-audio-bytes"))
	if !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRFQService_ReadFileContent_RefusesASpreadsheetThatIsACatalog(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	rows := strings.Repeat("Cemento,10\n", testRFQConfig().MaxSpreadsheetRows+1)

	format, _ := domain.AttachmentFormatFor("text/csv")
	_, _, err := h.service.readFileContent(context.Background(),
		domain.FileRFQDraftInput{
			Filename: "catalogo.csv",
			File:     domain.AttachmentUpload{ContentType: "text/csv"},
		}, format, []byte(rows))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestReadUpload_RefusesAFileOverTheLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		size     int64
		body     string
		maxBytes int64
		wantErr  error
	}{
		{"a declared size over the limit", 100, "short", 10, domain.ErrTooLarge},
		// A client can understate the size, so the read is capped too.
		{"a body longer than its declared size", 5, "far longer than five bytes", 10, domain.ErrTooLarge},
		{"an empty file", 0, "", 10, domain.ErrInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := readUpload(domain.AttachmentUpload{
				Size:    tc.size,
				Content: strings.NewReader(tc.body),
			}, tc.maxBytes)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestRFQService_NormalizeFileRFQDraftInput_RefusesWhatCannotBeRead(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	h.service.WithFileIntake(&fakeAttachmentStore{}, &fakeTranscriber{}, 10<<20)

	t.Run("an unaccepted content type", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := h.service.normalizeFileRFQDraftInput(domain.FileRFQDraftInput{
			ChannelID: testChannelID,
			Filename:  "pedido.exe",
			File: domain.AttachmentUpload{
				ContentType: "application/x-msdownload",
				Size:        4,
				Content:     strings.NewReader("junk"),
			},
		})
		if domain.CodeOf(err) != domain.CodeUnsupportedFileType {
			t.Fatalf("code = %q, want UNSUPPORTED_FILE_TYPE", domain.CodeOf(err))
		}
	})

	t.Run("no channel", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := h.service.normalizeFileRFQDraftInput(domain.FileRFQDraftInput{
			Filename: "pedido.csv",
			File: domain.AttachmentUpload{
				ContentType: "text/csv", Size: 4, Content: strings.NewReader("a,b\n"),
			},
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("a filename with no extension takes the format's own", func(t *testing.T) {
		t.Parallel()
		normalized, format, _, err := h.service.normalizeFileRFQDraftInput(
			domain.FileRFQDraftInput{
				ChannelID: testChannelID,
				Filename:  "voicenote",
				File: domain.AttachmentUpload{
					ContentType: "audio/mp4", Size: 5, Content: strings.NewReader("bytes"),
				},
			})
		if err != nil {
			t.Fatalf("normalizeFileRFQDraftInput returned %v", err)
		}
		if format.Type != domain.AttachmentTypeAudio {
			t.Errorf("format = %q, want AUDIO", format.Type)
		}
		if normalized.Filename != "pedido.m4a" {
			t.Errorf("filename = %q, want the accepted format's extension", normalized.Filename)
		}
	})
}

func TestRFQService_CreateFileDraft_KeepsTheFileWhenTheModelReadsNothing(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	store := &fakeAttachmentStore{}
	h.service.WithFileIntake(store, &fakeTranscriber{}, 10<<20)

	draft, err := h.service.CreateFileDraft(context.Background(), rfqTenant(),
		domain.FileRFQDraftInput{
			ChannelID: testChannelID,
			Filename:  "pedido.csv",
			File: domain.AttachmentUpload{
				ContentType: "text/csv",
				Size:        20,
				Content:     strings.NewReader("Producto,Cantidad\nCemento,10\n"),
			},
		})
	if err != nil {
		t.Fatalf("CreateFileDraft returned %v", err)
	}
	if len(h.rfqs.created) != 1 {
		t.Fatalf("created %d RFQs, want the order kept", len(h.rfqs.created))
	}
	if store.calls != 1 {
		t.Fatalf("stored the file %d times, want 1", store.calls)
	}
	if store.extracted != "Producto\tCantidad\nCemento\t10" {
		t.Errorf("attachment kept %q, want what was read out of the file", store.extracted)
	}
	if draft.Quote != nil || len(draft.Items) != 0 {
		t.Errorf("draft = %+v, want the RFQ alone when no material was read", draft)
	}
}

func TestRFQService_CreateFileDraft_NamesTheFileWhenItYieldsNoText(t *testing.T) {
	t.Parallel()
	h := newRFQHarness(nil)
	h.service.WithFileIntake(&fakeAttachmentStore{}, &fakeTranscriber{}, 10<<20)

	if _, err := h.service.CreateFileDraft(context.Background(), rfqTenant(),
		domain.FileRFQDraftInput{
			ChannelID: testChannelID,
			Filename:  "pedido.jpg",
			File: domain.AttachmentUpload{
				ContentType: "image/jpeg",
				Size:        6,
				Content:     strings.NewReader("\xFF\xD8fake"),
			},
		}); err != nil {
		t.Fatalf("CreateFileDraft returned %v", err)
	}

	created := h.rfqs.created[0]
	if created.RawText == nil || !strings.Contains(*created.RawText, "pedido.jpg") {
		t.Errorf("raw text = %v, want it to name the file it arrived as", created.RawText)
	}
}

func TestRFQService_CreateFileDraft_MarksTheRFQFailedWhenTheModelDoesNot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		lines []domain.ExtractedRFQLine
		err   error
	}{
		// The two ways the pipeline ends with no quote. They differ in whether the model answered
		// at all, and in nothing the seller can act on: either way the order is theirs to load.
		{name: "the model fails", err: domain.ErrAIUnavailable},
		{name: "the model reads no material"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newRFQHarness(tc.lines)
			h.extractor.err = tc.err
			h.service.WithFileIntake(&fakeAttachmentStore{}, &fakeTranscriber{}, 10<<20)

			_, err := h.service.CreateFileDraft(context.Background(), rfqTenant(),
				domain.FileRFQDraftInput{
					ChannelID: testChannelID,
					Filename:  "pedido.csv",
					File: domain.AttachmentUpload{
						ContentType: "text/csv",
						Size:        20,
						Content:     strings.NewReader("Producto,Cantidad\nCemento,10\n"),
					},
				})
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("CreateFileDraft returned %v, want %v", err, tc.err)
			}
			if tc.err == nil && err != nil {
				t.Fatalf("CreateFileDraft returned %v", err)
			}

			// Left RECEIVED the row reads as still being processed, so the seller waits on a
			// pipeline that already gave up.
			if len(h.rfqs.updatedStatus) != 1 || h.rfqs.updatedStatus[0] != domain.RFQStatusFailed {
				t.Fatalf("wrote RFQ statuses %v, want one FAILED", h.rfqs.updatedStatus)
			}
			if len(h.rfqs.statusChanges) != 1 {
				t.Fatalf("appended %d status changes, want the transition recorded",
					len(h.rfqs.statusChanges))
			}
			change := h.rfqs.statusChanges[0]
			if change.newStatus != domain.RFQStatusFailed ||
				change.previousStatus == nil || *change.previousStatus != domain.RFQStatusReceived {
				t.Errorf("status change = %+v, want RECEIVED to FAILED", change)
			}
		})
	}
}
