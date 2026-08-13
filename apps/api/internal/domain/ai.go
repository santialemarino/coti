package domain

import (
	"context"
	"fmt"

	"github.com/pgvector/pgvector-go"
)

// EmbeddingDimension is the width every catalog vector has to be. product.embedding is
// VECTOR(1536), so a vector of any other length can neither be stored nor compared.
const EmbeddingDimension = 1536

// ContentKind is the kind of payload one block of a generation request carries.
type ContentKind string

const (
	ContentKindText     ContentKind = "TEXT"
	ContentKindImage    ContentKind = "IMAGE"
	ContentKindDocument ContentKind = "DOCUMENT"
)

// Content is one block of a generation request's input: a written message, a photo of a
// handwritten list, or an attached document.
type Content struct {
	Kind ContentKind
	// Text carries the block's words, and is empty on an IMAGE or a DOCUMENT.
	Text string
	// MediaType is the IANA type of Data, and is empty on a TEXT block.
	MediaType string
	Data      []byte
}

// TextContent builds a written block.
func TextContent(text string) Content {
	return Content{Kind: ContentKindText, Text: text}
}

// ImageContent builds a picture block, such as a photographed materials list.
func ImageContent(mediaType string, data []byte) Content {
	return Content{Kind: ContentKindImage, MediaType: mediaType, Data: data}
}

// DocumentContent builds an attachment block, such as a PDF quote request.
func DocumentContent(mediaType string, data []byte) Content {
	return Content{Kind: ContentKindDocument, MediaType: mediaType, Data: data}
}

// GenerationRequest is one call to a language model whose answer is constrained by a schema.
type GenerationRequest struct {
	// Instructions is the stable half of the prompt — the action catalog, the rules, the
	// pipeline — and goes first so a provider can cache the prefix across calls.
	Instructions string
	// Input is the variable half: the message, attachment or slice of state this call is about.
	Input []Content
	// Schema is the JSON Schema the answer has to satisfy. It carries the closed enums and their
	// escape value, so "I cannot resolve this" is a structurally valid answer rather than an
	// invented one.
	Schema map[string]any
	// MaxTokens caps the whole answer, the model's own reasoning included. Zero takes the
	// configured default.
	MaxTokens int
}

// Validate reports a request that cannot be sent at all, so a caller's mistake fails before it
// costs a provider round trip.
func (r GenerationRequest) Validate() error {
	switch {
	case r.Instructions == "":
		return fmt.Errorf("%w: generation request carries no instructions", ErrInvalidInput)
	case len(r.Input) == 0:
		return fmt.Errorf("%w: generation request carries no input", ErrInvalidInput)
	case len(r.Schema) == 0:
		return fmt.Errorf("%w: generation request carries no schema", ErrInvalidInput)
	}
	for i, block := range r.Input {
		switch block.Kind {
		case ContentKindText:
			if block.Text == "" {
				return fmt.Errorf("%w: input block %d is empty text", ErrInvalidInput, i)
			}
		case ContentKindImage, ContentKindDocument:
			if len(block.Data) == 0 {
				return fmt.Errorf("%w: input block %d carries no data", ErrInvalidInput, i)
			}
			if block.MediaType == "" {
				return fmt.Errorf("%w: input block %d carries no media type", ErrInvalidInput, i)
			}
		default:
			return fmt.Errorf("%w: input block %d has unknown kind %q", ErrInvalidInput, i, block.Kind)
		}
	}
	return nil
}

// GenerationUsage is what one call consumed. Recorded per call so the pilot's AI spend can be
// measured per operation.
type GenerationUsage struct {
	Provider          string
	Model             string
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
}

// Audio is one recording to transcribe. Filename travels to the provider, which reads its
// extension to pick a decoder.
type Audio struct {
	Filename  string
	MediaType string
	Data      []byte
}

// Validate reports a recording that cannot be sent at all.
func (a Audio) Validate() error {
	switch {
	case a.Filename == "":
		return fmt.Errorf("%w: audio carries no filename", ErrInvalidInput)
	case a.MediaType == "":
		return fmt.Errorf("%w: audio carries no media type", ErrInvalidInput)
	case len(a.Data) == 0:
		return fmt.Errorf("%w: audio carries no data", ErrInvalidInput)
	}
	return nil
}

// StructuredGenerator asks a language model for one answer shaped by req.Schema and decodes it
// into out. Adapters live in internal/ai and are bound in the composition root; nothing below the
// port knows which provider is behind it.
type StructuredGenerator interface {
	Generate(ctx context.Context, req GenerationRequest, out any) (*GenerationUsage, error)
}

// Embedder turns text into EmbeddingDimension-wide vectors for semantic catalog search. The
// returned slice is index-aligned with texts.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error)
}

// Transcriber turns a recording into the text an RFQ can be extracted from.
type Transcriber interface {
	Transcribe(ctx context.Context, audio Audio) (string, error)
}
