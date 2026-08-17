package domain

import (
	"errors"
	"testing"
)

// schema stands in for a real one: Validate only cares that a schema was supplied.
var schema = map[string]any{"type": "object"}

func TestGenerationRequest_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		request GenerationRequest
		wantErr bool
	}{
		{
			name: "text input",
			request: GenerationRequest{
				Instructions: "map the message onto the action catalog",
				Input:        []Content{TextContent("300 bolsas de cemento")},
				Schema:       schema,
			},
		},
		{
			name: "photographed list",
			request: GenerationRequest{
				Instructions: "read the list in the photo",
				Input:        []Content{ImageContent("image/jpeg", []byte{0xFF, 0xD8})},
				Schema:       schema,
			},
		},
		{
			name: "no instructions",
			request: GenerationRequest{
				Input:  []Content{TextContent("300 bolsas de cemento")},
				Schema: schema,
			},
			wantErr: true,
		},
		{
			name: "no input",
			request: GenerationRequest{
				Instructions: "map the message onto the action catalog",
				Schema:       schema,
			},
			wantErr: true,
		},
		{
			name: "no schema",
			request: GenerationRequest{
				Instructions: "map the message onto the action catalog",
				Input:        []Content{TextContent("300 bolsas de cemento")},
			},
			wantErr: true,
		},
		{
			name: "empty text block",
			request: GenerationRequest{
				Instructions: "map the message onto the action catalog",
				Input:        []Content{TextContent("")},
				Schema:       schema,
			},
			wantErr: true,
		},
		{
			name: "image without data",
			request: GenerationRequest{
				Instructions: "read the list in the photo",
				Input:        []Content{ImageContent("image/jpeg", nil)},
				Schema:       schema,
			},
			wantErr: true,
		},
		{
			name: "document without a media type",
			request: GenerationRequest{
				Instructions: "read the attached request",
				Input:        []Content{DocumentContent("", []byte("%PDF-1.7"))},
				Schema:       schema,
			},
			wantErr: true,
		},
		{
			name: "unknown kind",
			request: GenerationRequest{
				Instructions: "map the message onto the action catalog",
				Input:        []Content{{Kind: "VIDEO", Data: []byte{0x00}, MediaType: "video/mp4"}},
				Schema:       schema,
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.request.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("Validate() = %v, want it to match ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestAudio_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		audio   Audio
		wantErr bool
	}{
		{
			name:  "complete",
			audio: Audio{Filename: "nota.ogg", MediaType: "audio/ogg", Data: []byte("OggS")},
		},
		{
			name:    "no filename",
			audio:   Audio{MediaType: "audio/ogg", Data: []byte("OggS")},
			wantErr: true,
		},
		{
			name:    "no media type",
			audio:   Audio{Filename: "nota.ogg", Data: []byte("OggS")},
			wantErr: true,
		},
		{
			name:    "no data",
			audio:   Audio{Filename: "nota.ogg", MediaType: "audio/ogg"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.audio.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("Validate() = %v, want it to match ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// The dimension is what ties the port to the catalog column. It is asserted on its own because
// changing it means an ALTER on product.embedding and re-embedding the whole catalog.
func TestEmbeddingDimension_MatchesTheCatalogColumn(t *testing.T) {
	t.Parallel()

	if EmbeddingDimension != 1536 {
		t.Fatalf("EmbeddingDimension = %d, but product.embedding is VECTOR(1536)", EmbeddingDimension)
	}
}
