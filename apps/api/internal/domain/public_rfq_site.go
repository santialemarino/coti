package domain

import "github.com/google/uuid"

// PublicRFQSite is the supplier identity and active intake destinations of one hostname.
type PublicRFQSite struct {
	MaxTextCharacters    int
	MaxFileSize          int64
	AcceptedContentTypes []string
	SupplierName         string
	LogoURL              *string
	Branches             []PublicRFQSiteBranch
}

// PublicRFQSiteBranch identifies one available branch's public intake.
type PublicRFQSiteBranch struct {
	Name      string
	ChannelID uuid.UUID
}
