// Package branding retrieves untrusted public brand assets behind a restricted HTTP client.
package branding

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("ff00::/8"),
}

// LogoLoader downloads small public PNG or JPEG logos without ambient network authority.
type LogoLoader struct {
	client       *http.Client
	resolver     *net.Resolver
	maxSizeBytes int64
	maxPixels    int64
	maxRedirects int
}

// NewLogoLoader builds a loader with proxying disabled and address validation on every dial.
func NewLogoLoader(cfg config.QuoteLogoConfig) *LogoLoader {
	loader := &LogoLoader{resolver: net.DefaultResolver, maxSizeBytes: cfg.MaxSizeBytes,
		maxPixels: cfg.MaxPixels, maxRedirects: cfg.MaxRedirects}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = loader.dialContext
	transport.DisableCompression = true
	loader.client = &http.Client{Transport: transport, Timeout: cfg.FetchTimeout,
		CheckRedirect: loader.checkRedirect}
	return loader
}

// Load returns fully decoded, size-bounded image bytes.
func (l *LogoLoader) Load(ctx context.Context, rawURL string) (*domain.BrandLogo, error) {
	parsed, err := validateLogoURL(rawURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build logo request: %w", err)
	}
	request.Header.Set("Accept", "image/png, image/jpeg")
	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch logo failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch logo: unexpected status %d", response.StatusCode)
	}
	if response.ContentLength > l.maxSizeBytes {
		return nil, fmt.Errorf("logo exceeds maximum size")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, l.maxSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read logo: %w", err)
	}
	if int64(len(content)) > l.maxSizeBytes {
		return nil, fmt.Errorf("logo exceeds maximum size")
	}
	imageConfig, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, fmt.Errorf("logo is not a valid PNG or JPEG")
	}
	pixels := int64(imageConfig.Width) * int64(imageConfig.Height)
	if imageConfig.Width <= 0 || imageConfig.Height <= 0 || pixels > l.maxPixels {
		return nil, fmt.Errorf("logo exceeds maximum pixel count")
	}
	if _, _, err := image.Decode(bytes.NewReader(content)); err != nil {
		return nil, fmt.Errorf("logo cannot be decoded")
	}
	contentType := "image/png"
	if format == "jpeg" {
		contentType = "image/jpeg"
	}
	return &domain.BrandLogo{Bytes: content, ContentType: contentType}, nil
}

func (l *LogoLoader) checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > l.maxRedirects {
		return fmt.Errorf("logo redirect limit exceeded")
	}
	_, err := validateLogoURL(request.URL.String())
	return err
}

func (l *LogoLoader) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, fmt.Errorf("logo destination must use port 443")
	}
	addresses, err := l.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve logo host")
	}
	for _, resolved := range addresses {
		if !isPublicIP(resolved.IP) {
			return nil, fmt.Errorf("logo host resolved to a non-public address")
		}
	}
	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func validateLogoURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("logo URL must be absolute HTTPS")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("logo URL must not contain credentials")
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return nil, fmt.Errorf("logo URL must use port 443")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicIP(ip) {
		return nil, fmt.Errorf("logo URL names a non-public address")
	}
	return parsed, nil
}

func isPublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
