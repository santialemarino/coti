package domain

import "testing"

func TestNormalizeGrossIncomeRegistration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"exempt in any case", "  exento ", GrossIncomeExempt, true},
		{"plain number", "1234567", "1234567", true},
		{"hyphenated groups", "901-123456-7", "901-123456-7", true},
		{"dots and spaces removed", " 901.123 456-7 ", "901123456-7", true},
		{"a CUIT", "30-71234567-9", "30-71234567-9", true},
		{"too few digits", "12-34", "", false},
		{"too many digits", "1234567890123456", "", false},
		{"letters", "IIBB 1234567", "", false},
		{"leading hyphen", "-1234567", "", false},
		{"double hyphen", "123--4567", "", false},
		{"empty", "  ", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := NormalizeGrossIncomeRegistration(tc.raw)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("NormalizeGrossIncomeRegistration(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNormalizeFiscalAddress(t *testing.T) {
	t.Parallel()
	if got := NormalizeFiscalAddress("  Av.  Rivadavia\t 1234 \n Morón "); got != "Av. Rivadavia 1234 Morón" {
		t.Fatalf("NormalizeFiscalAddress() = %q", got)
	}
}

func TestNormalizeTaxID(t *testing.T) {
	t.Parallel()
	if got := NormalizeTaxID(" 30-71.234 567-9 "); got != "30712345679" {
		t.Fatalf("NormalizeTaxID() = %q", got)
	}
}
