package repository

import (
	"context"

	"github.com/google/uuid"
)

// ChannelRepository owns persistence for channel.
type ChannelRepository struct{}

// NewChannelRepository builds a ChannelRepository.
func NewChannelRepository() *ChannelRepository {
	return &ChannelRepository{}
}

// CreateManualEntry opens the branch's manual-entry channel, the one a counter, phone or
// unintegrated-messaging order arrives through. Every branch has exactly one, created with the
// branch, because rfq.channel_id is NOT NULL and a manual order has no other channel to point
// at. The conflict target is the partial index covering the identifier-less case.
func (r *ChannelRepository) CreateManualEntry(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) error {
	_, err := q.Exec(ctx,
		`INSERT INTO channel (account_id, branch_id, type)
		 VALUES ($1, $2, 'MANUAL_ENTRY')
		 ON CONFLICT (branch_id, type) WHERE identifier IS NULL DO NOTHING`,
		accountID, branchID)
	return err
}
