package services

import (
	"context"
	"net"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetSite resolves the supplier and active branches registered for an exact hostname.
func (s *PublicRFQService) GetSite(ctx context.Context, hostname string) (*domain.PublicRFQSite, error) {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	if len(hostname) < 3 || len(hostname) > 253 || strings.ContainsAny(hostname, "/:@ \\?#") || net.ParseIP(hostname) != nil {
		return nil, domain.ErrNotFound
	}
	site, err := s.destinations.GetSiteCrossAccount(ctx, s.db.CrossAccount(), hostname)
	if err != nil {
		return nil, err
	}
	site.MaxTextCharacters = s.cfg.MaxTextCharacters
	site.MaxFileSize = s.maxFileSize
	site.AcceptedContentTypes = domain.AcceptedAttachmentContentTypes()
	return site, nil
}
