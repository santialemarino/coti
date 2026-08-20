package domain

import (
	"errors"
	"testing"
)

func TestNormalizeChannelIdentifier(t *testing.T) {
	t.Parallel()
	blank := "   "
	padded := "  +5491100000000  "
	number := "+5491100000000"

	for _, test := range []struct {
		name  string
		given *string
		want  *string
	}{
		{name: "absent", given: nil, want: nil},
		{name: "blank becomes absent", given: &blank, want: nil},
		{name: "trimmed", given: &padded, want: &number},
		{name: "kept", given: &number, want: &number},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeChannelIdentifier(test.given)
			switch {
			case test.want == nil && got != nil:
				t.Fatalf("NormalizeChannelIdentifier() = %q, want nil", *got)
			case test.want == nil:
			case got == nil:
				t.Fatalf("NormalizeChannelIdentifier() = nil, want %q", *test.want)
			case *got != *test.want:
				t.Fatalf("NormalizeChannelIdentifier() = %q, want %q", *got, *test.want)
			}
		})
	}
}

func TestValidateChannelIdentifier(t *testing.T) {
	t.Parallel()
	number := "+5491100000000"

	for _, test := range []struct {
		name        string
		channelType ChannelType
		identifier  *string
		wantRefused bool
	}{
		{name: "whatsapp with a number", channelType: ChannelTypeWhatsApp, identifier: &number},
		{name: "whatsapp without one", channelType: ChannelTypeWhatsApp},
		{name: "email with a mailbox", channelType: ChannelTypeEmail, identifier: &number},
		{name: "webapp without one", channelType: ChannelTypeWebApp},
		{name: "manual entry without one", channelType: ChannelTypeManualEntry},
		{name: "webapp with one", channelType: ChannelTypeWebApp, identifier: &number,
			wantRefused: true},
		{name: "manual entry with one", channelType: ChannelTypeManualEntry, identifier: &number,
			wantRefused: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateChannelIdentifier(test.channelType, test.identifier)
			if !test.wantRefused {
				if err != nil {
					t.Fatalf("ValidateChannelIdentifier() = %v, want no error", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ValidateChannelIdentifier() = %v, want %v", err, ErrInvalidInput)
			}
			if CodeOf(err) != CodeChannelIdentifier {
				t.Errorf("CodeOf() = %v, want %v", CodeOf(err), CodeChannelIdentifier)
			}
		})
	}
}

func TestIsValidChannelType(t *testing.T) {
	t.Parallel()

	for _, channelType := range channelTypes {
		if !IsValidChannelType(channelType) {
			t.Errorf("IsValidChannelType(%s) = false, want true", channelType)
		}
	}
	for _, channelType := range []ChannelType{"", "whatsapp", "TELEGRAM", "COUNTER"} {
		if IsValidChannelType(channelType) {
			t.Errorf("IsValidChannelType(%q) = true, want false", channelType)
		}
	}
}

func TestChannelTypesMatchTheDatabaseEnum(t *testing.T) {
	t.Parallel()
	// Written out rather than read from the constants: these strings are channel_type's labels,
	// and a Go rename that drifts from the enum fails every write with an invalid-input error
	// from Postgres rather than at the boundary that was supposed to catch it.
	want := []string{"WHATSAPP", "EMAIL", "WEBAPP", "MANUAL_ENTRY"}

	if len(channelTypes) != len(want) {
		t.Fatalf("channelTypes = %v, want the enum's %v", channelTypes, want)
	}
	for i, label := range want {
		if string(channelTypes[i]) != label {
			t.Errorf("channelTypes[%d] = %q, want %q", i, channelTypes[i], label)
		}
	}
}
