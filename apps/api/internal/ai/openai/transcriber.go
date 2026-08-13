package openai

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.Transcriber = (*Transcriber)(nil)

// Transcriber turns a voice note into the text an RFQ can be extracted from.
type Transcriber struct {
	client *http.Client
	cfg    config.AIConfig
	log    *slog.Logger
}

// NewTranscriber builds a Transcriber from the AI configuration.
func NewTranscriber(cfg config.AIConfig, log *slog.Logger) *Transcriber {
	return &Transcriber{
		client: &http.Client{Timeout: cfg.TranscriptionTimeout},
		cfg:    cfg,
		log:    log,
	}
}

// transcriptionReply is the answer to POST /audio/transcriptions asked for as JSON.
type transcriptionReply struct {
	Text string `json:"text"`
}

// Transcribe uploads the recording and returns what was said.
func (t *Transcriber) Transcribe(ctx context.Context, audio domain.Audio) (string, error) {
	if err := audio.Validate(); err != nil {
		return "", err
	}

	var text string
	started := time.Now()
	attempts, err := ai.Retry(ctx, t.cfg.Retry, func(ctx context.Context) error {
		// Rebuilt per attempt: the body is a reader the previous attempt consumed.
		body, contentType, err := t.multipartBody(audio)
		if err != nil {
			return err
		}
		request, err := newRequest(ctx, t.cfg.OpenAIBaseURL, t.cfg.OpenAIAPIKey,
			"/audio/transcriptions", contentType, body)
		if err != nil {
			return err
		}

		var reply transcriptionReply
		if err := send(t.client, request, &reply); err != nil {
			return err
		}
		if strings.TrimSpace(reply.Text) == "" {
			return ai.Retryable(errEmptyReply)
		}
		text = reply.Text
		return nil
	})
	ai.LogCall(ctx, t.log, ai.Call{
		Provider:  string(config.AIProviderOpenAI),
		Model:     t.cfg.TranscriptionModel,
		Operation: "transcribe",
		Attempts:  attempts,
		Elapsed:   time.Since(started),
	}, err)
	if err != nil {
		return "", ai.Unavailable(err)
	}
	return text, nil
}

// multipartBody composes the upload. The filename travels with it because the provider reads the
// extension to pick a decoder, and the media type is declared rather than guessed from the bytes.
func (t *Transcriber) multipartBody(audio domain.Audio) (*bytes.Reader, string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	if err := form.WriteField("model", t.cfg.TranscriptionModel); err != nil {
		return nil, "", fmt.Errorf("compose transcription request: %w", err)
	}
	if err := form.WriteField("response_format", "json"); err != nil {
		return nil, "", fmt.Errorf("compose transcription request: %w", err)
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, audio.Filename))
	header.Set("Content-Type", audio.MediaType)
	file, err := form.CreatePart(header)
	if err != nil {
		return nil, "", fmt.Errorf("compose transcription request: %w", err)
	}
	if _, err := file.Write(audio.Data); err != nil {
		return nil, "", fmt.Errorf("compose transcription request: %w", err)
	}
	if err := form.Close(); err != nil {
		return nil, "", fmt.Errorf("compose transcription request: %w", err)
	}
	return bytes.NewReader(body.Bytes()), form.FormDataContentType(), nil
}
