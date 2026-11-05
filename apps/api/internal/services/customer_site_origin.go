package services

import (
	"net/url"
	"strings"
)

func customerSiteOrigin(hostname, localOrigin string) string {
	if localOrigin == "" {
		return "https://" + hostname
	}
	origin := strings.TrimRight(localOrigin, "/")
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
