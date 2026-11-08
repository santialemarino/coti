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
