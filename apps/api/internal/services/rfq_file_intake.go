package services

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

// spreadsheetCellSeparator joins a row's cells for the model. A tab survives commas and
// semicolons inside a cell, which a client's own description regularly carries.
const spreadsheetCellSeparator = "\t"

/*
 * readFileContent turns one uploaded order into the blocks the extractor reads. An image and a
 * PDF reach the model as they are, because their layout is what a materials table lives in; a
 * recording and a spreadsheet have no layout a model can use, so they are turned into text
 * first — by the transcriber and by the spreadsheet reader.
 *
 * The extracted text is returned alongside so the attachment row can keep what was read out of
 * the file, which is what makes an ingest reviewable after the fact.
 */
func (s *RFQService) readFileContent(
	ctx context.Context, in domain.FileRFQDraftInput, format domain.AttachmentFormat, data []byte,
) (blocks []domain.Content, extracted string, err error) {
	switch format.Type {
	case domain.AttachmentTypeImage:
		return []domain.Content{domain.ImageContent(in.File.ContentType, data)}, "", nil

	case domain.AttachmentTypePDF:
		return []domain.Content{domain.DocumentContent(in.File.ContentType, data)}, "", nil

	case domain.AttachmentTypeText:
		text, textErr := s.requiredRFQText(string(data))
		if textErr != nil {
			return nil, "", textErr
		}
		return []domain.Content{domain.TextContent(text)}, text, nil

	case domain.AttachmentTypeSpreadsheet:
		text, sheetErr := s.readSpreadsheet(in.Filename, data)
		if sheetErr != nil {
			return nil, "", sheetErr
		}
		return []domain.Content{domain.TextContent(text)}, text, nil

	case domain.AttachmentTypeAudio:
		text, audioErr := s.transcribe(ctx, in, data)
		if audioErr != nil {
			return nil, "", audioErr
		}
		return []domain.Content{domain.TextContent(text)}, text, nil

	default:
		return nil, "", domain.WithCode(domain.CodeUnsupportedFileType,
			fmt.Errorf("%w: %q cannot be read as an order", domain.ErrInvalidInput, format.Type))
	}
}

func (s *RFQService) readSpreadsheet(filename string, data []byte) (string, error) {
	rows, err := spreadsheet.ReadRaw(filename, strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("%w: the spreadsheet could not be read: %s",
			domain.ErrInvalidInput, err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: the spreadsheet has no rows", domain.ErrInvalidInput)
	}
	if len(rows) > s.cfg.MaxSpreadsheetRows {
		return "", fmt.Errorf("%w: the spreadsheet has %d rows and the limit is %d, which is a "+
			"catalog rather than an order", domain.ErrInvalidInput, len(rows),
			s.cfg.MaxSpreadsheetRows)
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, strings.Join(row, spreadsheetCellSeparator))
	}
	return s.requiredRFQText(strings.Join(lines, "\n"))
}

func (s *RFQService) transcribe(
	ctx context.Context, in domain.FileRFQDraftInput, data []byte,
) (string, error) {
	if s.transcriber == nil {
		return "", domain.ErrNotConfigured
	}
	audio := domain.Audio{
		Filename:  in.Filename,
		MediaType: in.File.ContentType,
		Data:      data,
	}
	if err := audio.Validate(); err != nil {
		return "", err
	}
	text, err := s.transcriber.Transcribe(ctx, audio)
	if err != nil {
		return "", err
	}
	// A recording with nothing said in it produces an empty transcript rather than an error,
	// and an empty order is not something the model should be asked to read.
	return s.requiredRFQText(text)
}

// readUpload buffers the file, refusing one over the limit while it is still arriving. The
// transports below all need the whole file in memory — an image block is base64, a transcript
// is a multipart body — so there is nothing to stream it to.
func readUpload(file domain.AttachmentUpload, maxBytes int64) ([]byte, error) {
	if file.Size > maxBytes {
		return nil, fmt.Errorf("%w: the file is %d bytes and the limit is %d",
			domain.ErrTooLarge, file.Size, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file.Content, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: the file is over the %d byte limit",
			domain.ErrTooLarge, maxBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: the file is empty", domain.ErrInvalidInput)
	}
	return data, nil
}
