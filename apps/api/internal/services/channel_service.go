package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// channelStore is the channel surface the service needs.
type channelStore interface {
	ListActiveByBranch(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) ([]domain.Channel, error)
	ListAllByBranch(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) ([]domain.Channel, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID) (*domain.Channel, error)
	Create(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, in domain.NewChannel) (*domain.Channel, error)
	Update(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID, in domain.ChannelUpdate) (*domain.Channel, error)
	Deactivate(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID) error
}

// configSealer encrypts a channel credential on its way to the database. There is no matching read:
// the API never returns a credential, so a configuration is replaced whole rather than edited.
type configSealer interface {
	Enabled() bool
	Seal(plaintext string) (string, error)
}

// ChannelService owns the intake channels of a branch.
type ChannelService struct {
	db       tenantTxRunner
	channels channelStore
	sealer   configSealer
}

// NewChannelService builds a ChannelService.
func NewChannelService(db tenantTxRunner, channels channelStore, sealer configSealer) *ChannelService {
	return &ChannelService{db: db, channels: channels, sealer: sealer}
}

// ListChannels returns the active intake channels of the selected branch.
func (s *ChannelService) ListChannels(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.Channel, error) {
	if err := requireBranch(tenant, "channel list"); err != nil {
		return nil, err
	}

	var channels []domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		channels, listErr = s.channels.ListActiveByBranch(ctx, q, tenant.AccountID, tenant.BranchID)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return channels, nil
}

// ListAllChannels returns every channel of the selected branch, closed ones included, so an
// administrator can reopen one. A closed channel is absent from every other read — it is not a
// route anything may arrive through — which is why administering them needs its own list.
func (s *ChannelService) ListAllChannels(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.Channel, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if err := requireBranch(tenant, "channel list"); err != nil {
		return nil, err
	}

	var channels []domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		channels, listErr = s.channels.ListAllByBranch(ctx, q, tenant.AccountID, tenant.BranchID)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return channels, nil
}

// CreateChannel opens an intake channel on the selected branch.
func (s *ChannelService) CreateChannel(
	ctx context.Context, tenant domain.Tenant, in domain.NewChannel,
) (*domain.Channel, error) {
	if err := requireBranch(tenant, "a channel"); err != nil {
		return nil, err
	}
	if !domain.IsValidChannelType(in.Type) {
		return nil, fmt.Errorf("%w: unknown channel type %q", domain.ErrInvalidInput, in.Type)
	}
	in.Identifier = domain.NormalizeChannelIdentifier(in.Identifier)
	config, err := s.sealConfig(in.Type, in.Config)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateChannelIdentifier(in.Type, in.Identifier, config != nil); err != nil {
		return nil, err
	}
	in.Config = config

	var channel *domain.Channel
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		channel, createErr = s.channels.Create(ctx, q, tenant.AccountID, tenant.BranchID, in)
		return createErr
	}); err != nil {
		return nil, err
	}
	return channel, nil
}

// UpdateChannel replaces a channel's editable fields. A configuration sent with the request
// replaces the stored one whole; sending none leaves it untouched.
func (s *ChannelService) UpdateChannel(
	ctx context.Context, tenant domain.Tenant, channelID uuid.UUID, in domain.ChannelUpdate,
) (*domain.Channel, error) {
	if err := requireBranch(tenant, "a channel"); err != nil {
		return nil, err
	}
	in.Identifier = domain.NormalizeChannelIdentifier(in.Identifier)
	// Read outside the transaction: sealing writes back into in.Config, so a closure that ran a
	// second time would otherwise seal an envelope it had already sealed.
	requested := in.Config

	var channel *domain.Channel
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, getErr := s.channels.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
		if getErr != nil {
			return getErr
		}
		if in.IsActive != nil && !*in.IsActive {
			if err := assertChannelClosable(current); err != nil {
				return err
			}
		}
		sealed, sealErr := s.sealConfig(current.Type, requested)
		if sealErr != nil {
			return sealErr
		}
		// An absent config leaves the stored one alone; an explicit null or empty object is how a
		// caller removes it, and the two arrive here as nil and as "null".
		in.Config = sealed
		in.ClearConfig = sealed == nil && requested != nil
		// Validated against what the channel will hold, not what the request sent: dropping the
		// identifier off a channel whose stored configuration stays would orphan its credentials.
		configured := sealed != nil || (current.IsConfigured && !in.ClearConfig)
		if err := domain.ValidateChannelIdentifier(current.Type, in.Identifier,
			configured); err != nil {
			return err
		}

		var updateErr error
		channel, updateErr = s.channels.Update(ctx, q, tenant.AccountID, tenant.BranchID,
			channelID, in)
		return updateErr
	}); err != nil {
		return nil, err
	}
	return channel, nil
}

// DeactivateChannel closes a channel, refusing to close the branch's manual-entry route.
func (s *ChannelService) DeactivateChannel(
	ctx context.Context, tenant domain.Tenant, channelID uuid.UUID,
) error {
	if err := requireBranch(tenant, "a channel"); err != nil {
		return err
	}

	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, err := s.channels.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
		if err != nil {
			return err
		}
		if err := assertChannelClosable(current); err != nil {
			return err
		}
		return s.channels.Deactivate(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
	})
}

// sealConfig validates the requested settings against the channel type and returns them with every
// credential encrypted, or nil when the request carried none.
func (s *ChannelService) sealConfig(
	channelType domain.ChannelType, raw []byte,
) ([]byte, error) {
	config, err := domain.ParseChannelConfig(channelType, raw)
	if err != nil || config == nil {
		return nil, err
	}
	// Refused on the credential itself rather than on the config holding one, so the rule stays
	// true of a shape whose credentials are all optional.
	if err := config.MapSecrets(func(plaintext string) (string, error) {
		if !s.sealer.Enabled() {
			return "", fmt.Errorf("%w: CHANNEL_CONFIG_ENCRYPTION_KEY is unset, so a channel "+
				"credential cannot be stored", domain.ErrNotConfigured)
		}
		return s.sealer.Seal(plaintext)
	}); err != nil {
		return nil, err
	}
	return json.Marshal(config)
}

// assertChannelClosable refuses to close a branch's manual-entry channel: rfq.channel_id is NOT
// NULL and a counter, phone or unintegrated-messaging order has no other route to point at.
func assertChannelClosable(channel *domain.Channel) error {
	if channel.Type == domain.ChannelTypeManualEntry {
		return domain.WithCode(domain.CodeManualEntryChannel, fmt.Errorf(
			"%w: a branch's manual-entry channel cannot be closed", domain.ErrInvalidInput))
	}
	return nil
}
