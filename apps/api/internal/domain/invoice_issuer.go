package domain

import (
	"regexp"
	"strings"
)

// The point-of-sale range ARCA accepts for electronic invoicing.
const (
	PointOfSaleMin = 1
	PointOfSaleMax = 99998
)

// GrossIncomeExempt is the stored value of an issuer exempt from gross income tax.
const GrossIncomeExempt = "EXENTO"

// Province registrations differ in shape, so only the bounds every one of them fits are enforced.
const (
	grossIncomeDigitsMin = 5
	grossIncomeDigitsMax = 15
)

var grossIncomeNumber = regexp.MustCompile(`^[0-9]+(-[0-9]+)*$`)

// NormalizeGrossIncomeRegistration returns the canonical gross income registration: GrossIncomeExempt,
// or digit groups joined by single hyphens with spaces and dots removed. ok is false for anything else.
func NormalizeGrossIncomeRegistration(raw string) (value string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if strings.EqualFold(trimmed, GrossIncomeExempt) {
		return GrossIncomeExempt, true
	}
	number := strings.NewReplacer(" ", "", ".", "").Replace(trimmed)
	if !grossIncomeNumber.MatchString(number) {
		return "", false
	}
	digits := len(strings.ReplaceAll(number, "-", ""))
	if digits < grossIncomeDigitsMin || digits > grossIncomeDigitsMax {
		return "", false
	}
	return number, true
}

// NormalizeFiscalAddress trims an address and collapses every run of whitespace inside it.
func NormalizeFiscalAddress(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

// NormalizeTaxID strips the separators a CUIT is usually written with.
func NormalizeTaxID(raw string) string {
	return strings.NewReplacer("-", "", " ", "", ".", "").Replace(strings.TrimSpace(raw))
}
