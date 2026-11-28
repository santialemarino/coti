package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestNormalizeClientNotes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
		err   bool
	}{
		{"blank clears", " \n ", "", false},
		{"preserves line breaks", " First line\nSecond line  ", "First line\nSecond line", false},
		{"counts runes", strings.Repeat("ñ", 512), strings.Repeat("ñ", 512), false},
		{"rejects 513 runes", strings.Repeat("ñ", 513), "", true},
		{"rejects tab", "\ttext", "", true},
		{"rejects carriage return", "one\rtwo", "", true},
		{"rejects C1 controls", "one\u0085two", "", true},
		{"rejects NUL", "one\x00two", "", true},
		{"rejects invalid UTF-8", string([]byte{0xff}), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeClientNotes(tc.input)
			if tc.err {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || (tc.want == "" && got != nil) ||
				(tc.want != "" && (got == nil || *got != tc.want)) {
				t.Fatalf("notes = %v, error = %v, want %q", got, err, tc.want)
			}
		})
	}
}
