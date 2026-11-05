package domain

import "testing"

func TestMaskContact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		kind  ChannelType
		value string
		want  string
	}{
		{"argentine mobile", ChannelTypeWhatsApp, "+5491155557777", "+54 9 ••• ••••-7777"},
		{"foreign phone hides its country", ChannelTypeWhatsApp, "+34600111222", "••• ••••-1222"},
		{"short phone", ChannelTypeWhatsApp, "+1234", "•••"},
		{"email", ChannelTypeEmail, "juan.perez@gmail.com", "j•••@gmail.com"},
		{"two-character local part hides its initial", ChannelTypeEmail, "ab@corralon.test", "•••@corralon.test"},
		{"malformed email", ChannelTypeEmail, "nobody", "•••"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := MaskContact(tc.kind, tc.value); got != tc.want {
				t.Fatalf("MaskContact(%s, %q) = %q, want %q", tc.kind, tc.value, got, tc.want)
			}
		})
	}
}
