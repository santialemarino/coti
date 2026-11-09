package services

import (
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func customerSiteOrigin(hostname, localOrigin string) string {
	if localOrigin == "" {
		return "https://" + hostname
	}
	origin := strings.TrimRight(localOrigin, "/")
	base, err := url.Parse(origin)
	if err == nil && base.Hostname() != "" {
		label := strings.TrimSuffix(hostname, "."+base.Hostname())
		if normalized, err := domain.NormalizeSubdomain(label); err == nil && normalized+"."+base.Hostname() == hostname {
			return origin + "/c/" + normalized
		}
	}
	if strings.HasSuffix(hostname, ".localhost") {
		local, err := url.Parse(origin)
		if err == nil && local.Scheme == "http" && (local.Hostname() == "localhost" || local.Hostname() == "127.0.0.1") {
			port := local.Port()
			local.Host = hostname
			if port != "" {
				local.Host += ":" + port
			}
			return local.String()
		}
	}
	return "https://" + hostname
}
