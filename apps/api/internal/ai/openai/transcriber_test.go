package openai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func newTranscriber(srv *httptest.Server, attempts int) *Transcriber {
	return NewTranscriber(settings(srv, attempts), slog.New(slog.DiscardHandler))
}

func voiceNote() domain.Audio {
	return domain.Audio{
		Filename:  "pedido.ogg",
		MediaType: "audio/ogg",
		Data:      []byte("OggS-audio-bytes"),
	}
}

func TestTranscriber_ReturnsWhatWasSaid(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, `{"text":"necesito 300 bolsas de cemento"}`})

	text, err := newTranscriber(srv, 3).Transcribe(context.Background(), voiceNote())
	if err != nil {
		t.Fatalf("Transcribe() = %v, want nil", err)
	}
	if text != "necesito 300 bolsas de cemento" {
		t.Fatalf("text = %q, want the transcription", text)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1", provider.calls())
	}
}

// The provider reads the filename's extension to pick a decoder, so the upload has to carry it
// along with the declared media type — neither is guessed from the bytes.
func TestTranscriber_UploadsTheRecordingAsMultipart(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, `{"text":"necesito 300 bolsas"}`})

	audio := voiceNote()
	if _, err := newTranscriber(srv, 3).Transcribe(context.Background(), audio); err != nil {
		t.Fatalf("Transcribe() = %v, want nil", err)
	}

	contentType := provider.requests[0].Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("Content-Type = %q, which does not parse: %v", contentType, err)
	}
	form := multipart.NewReader(bytes.NewReader(provider.uploads[0]), params["boundary"])

	fields := map[string]string{}
	var (
		uploadedName string
		uploadedType string
		uploaded     []byte
	)
	for {
		part, err := form.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading the upload: %v", err)
		}
		body, _ := io.ReadAll(part)
		if part.FileName() != "" {
			uploadedName = part.FileName()
			uploadedType = part.Header.Get("Content-Type")
			uploaded = body
			continue
		}
		fields[part.FormName()] = string(body)
	}

	if uploadedName != audio.Filename {
		t.Fatalf("uploaded filename = %q, want %q", uploadedName, audio.Filename)
	}
	if uploadedType != audio.MediaType {
		t.Fatalf("uploaded media type = %q, want %q", uploadedType, audio.MediaType)
	}
	if !bytes.Equal(uploaded, audio.Data) {
		t.Fatalf("uploaded %q, want the recording's bytes", uploaded)
	}
	if fields["model"] != "whisper-1" {
		t.Fatalf("model = %q, want the configured one", fields["model"])
	}
	if fields["response_format"] != "json" {
		t.Fatalf("response_format = %q, want json", fields["response_format"])
	}
}

// The body is a reader the first attempt consumes, so a retry has to compose it again. Reusing it
// would upload an empty file on every attempt after the first.
func TestTranscriber_RebuildsTheUploadOnEachAttempt(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t,
		reply{http.StatusBadGateway, `{"error":{"message":"later"}}`},
		reply{http.StatusOK, `{"text":"necesito 300 bolsas"}`},
	)

	audio := voiceNote()
	if _, err := newTranscriber(srv, 3).Transcribe(context.Background(), audio); err != nil {
		t.Fatalf("Transcribe() = %v, want nil", err)
	}
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls())
	}
	for attempt, upload := range provider.uploads {
		if !bytes.Contains(upload, audio.Data) {
			t.Fatalf("attempt %d uploaded no audio: %q", attempt+1, upload)
		}
	}
}

func TestTranscriber_RetriesAnEmptyTranscription(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t,
		reply{http.StatusOK, `{"text":"   "}`},
		reply{http.StatusOK, `{"text":"necesito 300 bolsas"}`},
	)

	text, err := newTranscriber(srv, 3).Transcribe(context.Background(), voiceNote())
	if err != nil {
		t.Fatalf("Transcribe() = %v, want nil", err)
	}
	if text != "necesito 300 bolsas" {
		t.Fatalf("text = %q, want the second answer", text)
	}
	if provider.calls() != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls())
	}
}

func TestTranscriber_DoesNotRepeatARejectedRequest(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusBadRequest, `{"error":{"message":"unsupported format"}}`})

	_, err := newTranscriber(srv, 3).Transcribe(context.Background(), voiceNote())
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Transcribe() = %v, want it to match domain.ErrAIUnavailable", err)
	}
	if provider.calls() != 1 {
		t.Fatalf("calls = %d, want 1", provider.calls())
	}
}

func TestTranscriber_RejectsAnUnsendableRecordingWithoutCallingTheProvider(t *testing.T) {
	t.Parallel()

	srv, provider := serve(t, reply{http.StatusOK, `{"text":"never asked for"}`})

	_, err := newTranscriber(srv, 3).Transcribe(context.Background(),
		domain.Audio{Filename: "pedido.ogg", MediaType: "audio/ogg"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Transcribe() = %v, want it to match domain.ErrInvalidInput", err)
	}
	if provider.calls() != 0 {
		t.Fatalf("calls = %d, want 0", provider.calls())
	}
}
