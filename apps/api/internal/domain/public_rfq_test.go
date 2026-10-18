package domain

import (
	"errors"
	"testing"
)

func TestPublicRFQContactNormalize(t *testing.T) {
	contact := PublicRFQContact{TaxID: "20-12345678-6", Name: "  Obra Norte  ", Email: " CONTACTO@example.com ", Phone: " +54 11 5555-5555 "}
	normalized, err := contact.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.TaxID != "20123456786" || normalized.Name != "Obra Norte" || normalized.Email != "contacto@example.com" || normalized.Phone != "+54 11 5555-5555" {
		t.Fatalf("unexpected normalized contact: %#v", normalized)
	}
	contact.Phone = ""
	if _, err := contact.Normalize(); err != nil {
		t.Fatalf("phone must be optional: %v", err)
	}
}

func TestPublicRFQContactRejectsInvalidDeclarations(t *testing.T) {
	valid := PublicRFQContact{TaxID: "20123456786", Name: "Obra Norte", Email: "contacto@example.com"}
	tests := map[string]func(*PublicRFQContact){
		"missing name":       func(c *PublicRFQContact) { c.Name = " " },
		"missing email":      func(c *PublicRFQContact) { c.Email = "" },
		"display name email": func(c *PublicRFQContact) { c.Email = "Other <contacto@example.com>" },
		"invalid checksum":   func(c *PublicRFQContact) { c.TaxID = "20123456780" },
		"zero CUIT":          func(c *PublicRFQContact) { c.TaxID = "00000000000" },
		"non-digit CUIT":     func(c *PublicRFQContact) { c.TaxID = "2012345678x" },
		"control character":  func(c *PublicRFQContact) { c.Phone = "123\n456" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			contact := valid
			mutate(&contact)
			if _, err := contact.Normalize(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want invalid input", err)
			}
		})
	}
}
