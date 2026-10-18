package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// PublicRFQContact is the customer-declared contact snapshot of one public request.
type PublicRFQContact struct {
	TaxID string `json:"tax_id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone,omitempty"`
}

// Normalize validates customer declarations without consulting or changing client profiles.
func (c PublicRFQContact) Normalize() (PublicRFQContact, error) {
	c.TaxID = NormalizeTaxID(c.TaxID)
	c.Name = strings.TrimSpace(c.Name)
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.Phone = strings.TrimSpace(c.Phone)
	if !ValidCUIT(c.TaxID) {
		return c, fmt.Errorf("%w: invalid CUIT", ErrInvalidInput)
	}
	if c.Name == "" || utf8.RuneCountInString(c.Name) > 255 || utf8.RuneCountInString(c.Email) > 255 || utf8.RuneCountInString(c.Phone) > 64 {
		return c, fmt.Errorf("%w: invalid contact fields", ErrInvalidInput)
	}
	address, err := mail.ParseAddress(c.Email)
	if err != nil || address.Address != c.Email {
		return c, fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	for _, value := range []string{c.Name, c.Email, c.Phone} {
		if strings.ContainsFunc(value, unicode.IsControl) {
			return c, fmt.Errorf("%w: contact contains control characters", ErrInvalidInput)
		}
	}
	return c, nil
}

// ValidCUIT checks the eleven digits and checksum of an Argentine tax identifier.
func ValidCUIT(value string) bool {
	if len(value) != 11 {
		return false
	}
	weights := [...]int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
		if i < len(weights) {
			sum += int(value[i]-'0') * weights[i]
		}
	}
	check := 11 - sum%11
	if check == 11 {
		check = 0
	}
	if check == 10 {
		check = 9
	}
	return check == int(value[10]-'0') && value != "00000000000"
}

// PublicRFQDestination is the public identity and private routing of a WEBAPP channel.
type PublicRFQDestination struct {
	MaxTextCharacters    int
	MaxFileSize          int64
	AcceptedContentTypes []string
	Route                InboundChannelRoute
	SupplierName         string
	BranchName           string
	LogoURL              *string
}

// PublicRFQInput carries a single stateless submission and its retry identifier.
type PublicRFQInput struct {
	RequestID uuid.UUID
	Contact   PublicRFQContact
	Text      string
	File      *AttachmentUpload
	Filename  string
}
