package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

// spreadsheetSweepCellSeparator joins a row's cells. A tab survives commas and semicolons inside a
// cell, which a comma would not — the same reason the inline intake picks it.
const spreadsheetSweepCellSeparator = "\t"

/*
 * WithTranscription wires the voice-note reader. The sweep refuses a recording when it is unbound
 * rather than failing the attachment, because an unconfigured provider is the deployment's problem
 * and a later run will find it bound.
 */
func (s *RFQAttachmentService) WithTranscription(
	transcriber domain.Transcriber, maxTextCharacters int,
) *RFQAttachmentService {
	s.transcriber = transcriber
	s.maxTextCharacters = maxTextCharacters
	return s
}

/*
 * ReadStoredAttachment reads one already-stored file into the block a model reads and the text the
 * attachment row keeps. It is the sweep's half of the multi-format engine: the inline intake reads
 * a file while the request is still open, and this reads one that arrived on its own.
 *
 * An image and a PDF have no text step — their layout is what a materials table lives in, so they
 * go to the model as they are and the returned text is empty. A recording, a spreadsheet and a
 * text file become text first, which the row then keeps so an ingest stays reviewable.
 */
func (s *RFQAttachmentService) ReadStoredAttachment(
	ctx context.Context, attachment domain.ClaimedAttachment,
) (domain.Content, string, error) {
	if strings.TrimSpace(attachment.StorageKey) == "" {
		return domain.Content{}, "", fmt.Errorf("%w: attachment %s has no stored file",
			domain.ErrInvalidInput, attachment.ID)
	}

	object, err := s.storage.Download(ctx, attachment.StorageKey)
	if err != nil {
		return domain.Content{}, "", err
	}
	defer object.Body.Close()

	// The whole file, because every reader below needs it: an image block is base64, a transcript
	// is a multipart body, a spreadsheet parser seeks. The size was capped when the file was
	// accepted.
	data, err := io.ReadAll(object.Body)
	if err != nil {
		return domain.Content{}, "", err
	}

	// The key ends in the extension the format was accepted under, and both text readers below
	// pick their parser from it.
	filename := path.Base(attachment.StorageKey)

	switch attachment.Type {
	case domain.AttachmentTypeImage:
		return domain.ImageContent(object.ContentType, data), "", nil

	case domain.AttachmentTypePDF:
		return domain.DocumentContent(object.ContentType, data), "", nil

	case domain.AttachmentTypeText:
		return s.textBlock(string(data))

	case domain.AttachmentTypeSpreadsheet:
		rows, readErr := spreadsheet.ReadRaw(filename, bytes.NewReader(data))
		if readErr != nil {
			return domain.Content{}, "", fmt.Errorf("%w: the spreadsheet could not be read: %s",
				domain.ErrInvalidInput, readErr)
		}
		if len(rows) == 0 {
			return domain.Content{}, "", fmt.Errorf("%w: the spreadsheet has no rows",
				domain.ErrInvalidInput)
		}
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, strings.Join(row, spreadsheetSweepCellSeparator))
		}
		return s.textBlock(strings.Join(lines, "\n"))

	case domain.AttachmentTypeAudio:
		if s.transcriber == nil {
			return domain.Content{}, "", domain.ErrNotConfigured
		}
		audio := domain.Audio{
			Filename:  filename,
			MediaType: object.ContentType,
			Data:      data,
		}
		if validateErr := audio.Validate(); validateErr != nil {
			return domain.Content{}, "", validateErr
		}
		text, transcribeErr := s.transcriber.Transcribe(ctx, audio)
		if transcribeErr != nil {
			return domain.Content{}, "", transcribeErr
		}
		// A recording with nothing said in it transcribes to nothing rather than erroring, and an
		// empty order is not something to record as read.
		return s.textBlock(text)

	default:
		return domain.Content{}, "", fmt.Errorf("%w: %q cannot be read as an order",
			domain.ErrInvalidInput, attachment.Type)
	}
}

// textBlock bounds the text a format yielded and wraps it as the block the model reads.
func (s *RFQAttachmentService) textBlock(raw string) (domain.Content, string, error) {
	text, err := s.boundedText(raw)
	if err != nil {
		return domain.Content{}, "", err
	}
	return domain.TextContent(text), text, nil
}

// boundedText refuses an empty read and one past the pipeline's character cap, so a file that
// would be rejected by the extractor is closed out here instead of reaching it. An unset cap means
// this service was built for a path that reads no files — applying it would reject every read.
func (s *RFQAttachmentService) boundedText(raw string) (string, error) {
	text, err := requiredText(raw, "extracted_text")
	if err != nil {
		return "", err
	}
	if s.maxTextCharacters > 0 {
		if err := requireMaxRunes(text, "extracted_text", s.maxTextCharacters); err != nil {
			return "", err
		}
	}
	return text, nil
}
