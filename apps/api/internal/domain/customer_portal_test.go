package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDefaultPortalLabel(t *testing.T) {
	id := uuid.MustParse("3f2a9c1e-0000-4000-8000-000000000000")
	cases := []struct{ name, want string }{
		{"Corralón San Martín", "corralon-san-martin"},
		{"  Ñandú & Cía.  ", "nandu-cia"},
		{"Materiales--El   Águila!!", "materiales-el-aguila"},
		{"Marko", "marko"},
		{"Corralón 24", "corralon-24"},
		{strings.Repeat("ab ", 40), strings.TrimRight(strings.Repeat("ab-", 21), "-")},
		{"", "corralon-3f2a9c"},
		{"¡¡¡!!!", "corralon-3f2a9c"},
		{"WWW", "corralon-3f2a9c"},
		{"Admin", "corralon-3f2a9c"},
	}
	for _, tc := range cases {
		if got := DefaultPortalLabel(tc.name, id); got != tc.want {
			t.Errorf("DefaultPortalLabel(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPortalLabelCandidates(t *testing.T) {
	if got := PortalLabelCandidates("marko", 3); strings.Join(got, ",") != "marko,marko-2,marko-3" {
		t.Errorf("PortalLabelCandidates(marko) = %v", got)
	}
	full := strings.Repeat("a", 61) + "-b"
	for _, candidate := range PortalLabelCandidates(full, 12) {
		if _, err := NormalizeSubdomain(candidate); err != nil {
			t.Errorf("candidate %q is not a valid label: %v", candidate, err)
		}
	}
	if got := PortalLabelCandidates(full, 2)[1]; got != strings.Repeat("a", 61)+"-2" {
		t.Errorf("a full-length label takes its suffix as %q", got)
	}
}

func TestNormalizeSubdomain(t *testing.T) {
	for _, raw := range []string{"marko", " Marko ", "a", "corralon-12", strings.Repeat("a", 63)} {
		got, err := NormalizeSubdomain(raw)
		if err != nil || got != strings.ToLower(strings.TrimSpace(raw)) {
			t.Errorf("NormalizeSubdomain(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "www", " API ", "admin", "coti", "-marko", "marko-", "mark_o", "markó", "marko.localhost", "https://marko", "a/b", strings.Repeat("a", 64)} {
		if _, err := NormalizeSubdomain(raw); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("accepted %q: %v", raw, err)
		}
	}
}
