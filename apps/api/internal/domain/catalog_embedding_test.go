package domain

import "testing"

// description is nullable and reaches this in three shapes — absent, blank, and written — and
// only the last one belongs in the text sent to the model.
func TestProductEmbeddingInput_EmbeddingText(t *testing.T) {
	blank := "   "
	written := "  bolsa de 50kg "

	cases := []struct {
		name        string
		description *string
		want        string
	}{
		{"no description", nil, "Cemento Portland"},
		{"a blank description", &blank, "Cemento Portland"},
		{"a written description", &written, "Cemento Portland bolsa de 50kg"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := ProductEmbeddingInput{
				CanonicalName: " Cemento Portland ",
				Description:   tc.description,
			}
			if got := in.EmbeddingText(); got != tc.want {
				t.Errorf("EmbeddingText() = %q, want %q", got, tc.want)
			}
		})
	}
}
