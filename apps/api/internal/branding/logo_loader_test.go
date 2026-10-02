package branding

import (
	"net"
	"testing"
)

func TestValidateLogoURL_RequiresCredentialFreeHTTPSOnPort443(t *testing.T) {
	t.Parallel()
	valid := []string{"https://cdn.example.com/logo.png", "https://cdn.example.com:443/logo.jpg"}
	for _, raw := range valid {
		if _, err := validateLogoURL(raw); err != nil {
			t.Errorf("validateLogoURL(%q) = %v, want nil", raw, err)
		}
	}
	invalid := []string{"http://cdn.example.com/logo.png", "https://user:pass@example.com/logo.png",
		"https://example.com:8443/logo.png", "https://127.0.0.1/logo.png",
		"https://169.254.169.254/latest/meta-data"}
	for _, raw := range invalid {
		if _, err := validateLogoURL(raw); err == nil {
			t.Errorf("validateLogoURL(%q) = nil error", raw)
		}
	}
}

func TestIsPublicIP_RejectsPrivateReservedAndDocumentationNetworks(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"10.0.0.1", "127.0.0.1", "169.254.169.254", "192.168.1.1",
		"198.51.100.4", "203.0.113.8", "::1", "fc00::1", "2001:db8::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Errorf("isPublicIP(%s) = true", raw)
		}
	}
	if !isPublicIP(net.ParseIP("8.8.8.8")) {
		t.Error("isPublicIP(8.8.8.8) = false")
	}
}
