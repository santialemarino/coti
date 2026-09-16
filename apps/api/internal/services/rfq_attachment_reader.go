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
 * ReadStoredAttachmentText reads one already-stored file and returns the text its format carries.
 * It is the sweep's half of the multi-format engine: the inline intake reads a file while the
 * request is still open, and this reads one that arrived on its own.
 *
 * Only the formats that carry text reach here — a recording, a spreadsheet, a text file. An image
 * and a PDF are read by the model as they are, so there is no text to lift out of them without
 * running the extraction.
 */
func (s *RFQAttachmentService) ReadStoredAttachmentText(
	ctx context.Context, attachment domain.ClaimedAttachment,
) (string, error) {
	if strings.TrimSpace(attachment.StorageKey) == "" {
		return "", fmt.Errorf("%w: attachment %s has no stored file", domain.ErrInvalidInput,
			attachment.ID)
	}

	object, err := s.storage.Download(ctx, attachment.StorageKey)
	if err != nil {
		return "", err
	}
	defer object.Body.Close()

	// The whole file, because every reader below needs it: a transcript is a multipart body and a
	// spreadsheet parser seeks. The size was capped when the file was accepted.
	data, err := io.ReadAll(object.Body)
	if err != nil {
		return "", err
	}

	// The key ends in the extension the format was accepted under, and both readers below pick
	// their parser from it.
	filename := path.Base(attachment.StorageKey)

	switch attachment.Type {
	case domain.AttachmentTypeText:
		return s.boundedText(string(data))

	case domain.AttachmentTypeSpreadsheet:
		rows, readErr := spreadsheet.ReadRaw(filename, bytes.NewReader(data))
		if readErr != nil {
			return "", fmt.Errorf("%w: the spreadsheet could not be read: %s",
				domain.ErrInvalidInput, readErr)
		}
		if len(rows) == 0 {
			return "", fmt.Errorf("%w: the spreadsheet has no rows", domain.ErrInvalidInput)
		}
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			lines = append(lines, strings.Join(row, spreadsheetSweepCellSeparator))
		}
		return s.boundedText(strings.Join(lines, "\n"))

	case domain.AttachmentTypeAudio:
		if s.transcriber == nil {
			return "", domain.ErrNotConfigured
		}
		audio := domain.Audio{
			Filename:  filename,
			MediaType: object.ContentType,
			Data:      data,
		}
		if validateErr := audio.Validate(); validateErr != nil {
			return "", validateErr
		}
		text, transcribeErr := s.transcriber.Transcribe(ctx, audio)
		if transcribeErr != nil {
			return "", transcribeErr
		}
		// A recording with nothing said in it transcribes to nothing rather than erroring, and an
		// empty order is not something to record as read.
		return s.boundedText(text)

	default:
		return "", fmt.Errorf("%w: %q carries no text of its own", domain.ErrInvalidInput,
			attachment.Type)
	}
}

// boundedText refuses an empty read and one past the pipeline's character cap, so a file that
// would be rejected by the extractor is closed out here instead of reaching it.
func (s *RFQAttachmentService) boundedText(raw string) (string, error) {
	text, err := requiredText(raw, "extracted_text")
	if err != nil {
		return "", err
	}
	if err := requireMaxRunes(text, "extracted_text", s.maxTextCharacters); err != nil {
		return "", err
	}
	return text, nil
}
