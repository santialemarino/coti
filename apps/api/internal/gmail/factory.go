package gmail

import (
	"context"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Factory constructs Gmail mailbox readers using one application's OAuth settings.
type Factory struct {
	config config.GmailConfig
}

// NewFactory builds a Gmail reader factory.
func NewFactory(config config.GmailConfig) *Factory { return &Factory{config: config} }

// NewReader opens a mailbox using its decrypted offline credential.
func (f *Factory) NewReader(ctx context.Context, refreshToken string) (domain.EmailMailboxReader, error) {
	client, err := NewClient(ctx, f.config.ClientID, f.config.ClientSecret, refreshToken,
		f.config.RequestTimeout, f.config.MaxJSONBytes)
	if err != nil {
		return nil, err
	}
	return NewMailbox(client, f.config.MaxRawBytes, f.config.MaxRawBytes, f.config.MaxParts)
}
