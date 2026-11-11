package domain

import (
	"errors"
	"testing"
)

func TestNormalizeClientContact(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		kind        ChannelType
		value, want string
		valid       bool
	}{
		{"phone separators", ChannelTypeWhatsApp, " +54 9 (11) 5555-0101 ", "+5491155550101", true},
		{"email case", ChannelTypeEmail, " CUSTOMER@EXAMPLE.TEST ", "customer@example.test", true},
		{"missing country", ChannelTypeWhatsApp, "1155550101", "", false},
		{"missing phone", ChannelTypeWhatsApp, "", "", false},
		{"display name", ChannelTypeEmail, "Customer <customer@example.test>", "", false},
		{"control characters", ChannelTypeEmail, "customer@example.test\n", "", true},
		{"embedded control", ChannelTypeEmail, "customer\n@example.test", "", false},
		{"unsupported kind", ChannelTypeWebApp, "customer@example.test", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeClientContact(tc.kind, tc.value)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if tc.want != "" && got != tc.want {
					t.Fatalf("got %q, want %q", got, tc.want)
				}
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want invalid input", err)
			}
		})
	}
}

func TestNormalizeClientContacts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		contacts []ClientContact
		valid    bool
	}{
		{"empty", nil, true},
		{"one per kind", []ClientContact{{Kind: ChannelTypeEmail, Value: "one@example.test", Primary: true}, {Kind: ChannelTypeWhatsApp, Value: "+5491155550101", Primary: true}}, true},
		{"several emails", []ClientContact{{Kind: ChannelTypeEmail, Value: "one@example.test", Primary: true}, {Kind: ChannelTypeEmail, Value: "two@example.test"}}, true},
		{"normalized duplicate", []ClientContact{{Kind: ChannelTypeEmail, Value: "one@example.test", Primary: true}, {Kind: ChannelTypeEmail, Value: " ONE@EXAMPLE.TEST "}}, false},
		{"missing replacement", []ClientContact{{Kind: ChannelTypeEmail, Value: "one@example.test"}}, false},
		{"two primaries", []ClientContact{{Kind: ChannelTypeEmail, Value: "one@example.test", Primary: true}, {Kind: ChannelTypeEmail, Value: "two@example.test", Primary: true}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeClientContacts(tc.contacts)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestCanonicalPhone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, phone, want string }{
		{"buenos aires without the 9", "+54 11 5555-0101", "+5491155550101"},
		{"buenos aires with the 9", "+54 9 11 5555-0101", "+5491155550101"},
		{"interior area without the 9", "+54 (221) 555.0303", "+5492215550303"},
		{"interior area with the 9", "+549 221 555 0303", "+5492215550303"},
		{"too short to be argentine", "+54 11 5555", "+54115555"},
		{"another country", "+1 212 555 0100", "+12125550100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CanonicalPhone(tc.phone); got != tc.want {
				t.Fatalf("CanonicalPhone(%q) = %q, want %q", tc.phone, got, tc.want)
			}
		})
	}
}

func TestNormalizeClientContact_ArgentineMobileFormsMatch(t *testing.T) {
	t.Parallel()
	without, err := NormalizeClientContact(ChannelTypeWhatsApp, "+54 11 5555-0101")
	if err != nil {
		t.Fatal(err)
	}
	with, err := NormalizeClientContact(ChannelTypeWhatsApp, "+54 9 11 5555-0101")
	if err != nil {
		t.Fatal(err)
	}
	if without != with || with != "+5491155550101" {
		t.Fatalf("+54 11… normalized to %q and +54 9 11… to %q, want both +5491155550101", without, with)
	}
}
