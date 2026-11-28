package domain

import "testing"

func TestCustomerNextPath_KeepsOnlyPortalPages(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"/quotes/AbC_-123":           "/quotes/AbC_-123",
		"/orders":                    "/orders",
		"/orders/1b4e28ba-2fa1":      "/orders/1b4e28ba-2fa1",
		"/new-request":               "/new-request",
		"/profile":                   "/profile",
		"":                           "",
		"/quotes/":                   "",
		"/quotes/a/b":                "",
		"/quotes/a?delivery=b":       "",
		"//evil.test/quotes/a":       "",
		"https://evil.test/quotes/a": "",
		"/verify-contact":            "",
		"/quotes/a\n":                "",
	} {
		if got := CustomerNextPath(raw); got != want {
			t.Errorf("CustomerNextPath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestWhatsAppVerificationLink(t *testing.T) {
	t.Parallel()
	if got := WhatsAppVerificationLink("+54 9 11 4000-0291", "012345"); got != "https://wa.me/5491140000291?text=VERIFICAR%20012345" {
		t.Errorf("link = %q", got)
	}
	if got := WhatsAppVerificationLink("", "012345"); got != "" {
		t.Errorf("a channel without a number got a link: %q", got)
	}
}
