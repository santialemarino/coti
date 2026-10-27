package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const quoteSendColumns = `send.id, send.account_id, send.version_id, send.channel_id,
	channel.type, send.idempotency_key, COALESCE(send.destination, ''),
	send.provider_reference, COALESCE(send.public_token, ''), send.format,
	send.validity_days, send.sent_at, send.expires_at, send.tracking_status, send.created_at,
	channel.identifier, send.client_association_hash`

const quoteSendIdempotencyIndex = "uq_quote_send_idempotency_channel"

// QuoteSendRepository owns quote delivery attempts and public-token resolution.
type QuoteSendRepository struct{}

// NewQuoteSendRepository builds a QuoteSendRepository.
func NewQuoteSendRepository() *QuoteSendRepository { return &QuoteSendRepository{} }

// ListByOperation loads an idempotent send set only through its account, branch and quote.
func (r *QuoteSendRepository) ListByOperation(ctx context.Context, q Querier, accountID,
	branchID, quoteID, key uuid.UUID) ([]domain.QuoteSend, error) {
	rows, err := q.Query(ctx, `SELECT `+quoteSendColumns+`
		FROM quote_send send
		JOIN channel ON channel.account_id = send.account_id AND channel.id = send.channel_id
		JOIN quote_version version ON version.account_id = send.account_id AND version.id = send.version_id
		JOIN quote ON quote.account_id = send.account_id AND quote.id = version.quote_id
		WHERE send.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		  AND send.idempotency_key = $4
		ORDER BY channel.type, send.id`, accountID, branchID, quoteID, key)
	if err != nil {
		return nil, err
	}
	return scanQuoteSends(rows)
}

// CreateBatch inserts every selected channel in one statement.
func (r *QuoteSendRepository) CreateBatch(ctx context.Context, q Querier, accountID uuid.UUID,
	sends []domain.NewQuoteSend) ([]domain.QuoteSend, error) {
	payload, err := json.Marshal(sends)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `WITH incoming AS (
		  SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(
		    "ID" uuid, "VersionID" uuid, "ChannelID" uuid, "IdempotencyKey" uuid,
		    "Destination" text, "PublicToken" text, "Format" text, "ValidityDays" integer, "ClientAssociationHash" text)
		), inserted AS (
		  INSERT INTO quote_send (id, account_id, version_id, channel_id, idempotency_key,
		    destination, public_token, format, validity_days, client_association_hash)
		  SELECT incoming."ID", $1, version.id, channel.id, incoming."IdempotencyKey",
		    incoming."Destination", incoming."PublicToken", incoming."Format"::send_format,
		    incoming."ValidityDays", COALESCE(incoming."ClientAssociationHash",'')
		  FROM incoming
		  JOIN quote_version version ON version.account_id = $1
		    AND version.id = incoming."VersionID"
		  JOIN quote ON quote.account_id = $1 AND quote.id = version.quote_id
		  JOIN channel ON channel.account_id = $1 AND channel.id = incoming."ChannelID"
		    AND channel.branch_id = quote.branch_id
		  RETURNING *
		)
		SELECT `+quoteSendColumns+` FROM inserted send
		JOIN channel ON channel.account_id = send.account_id AND channel.id = send.channel_id
		ORDER BY channel.type, send.id`, accountID, payload)
	if isUniqueViolation(err, quoteSendIdempotencyIndex) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	created, err := scanQuoteSends(rows)
	if isUniqueViolation(err, quoteSendIdempotencyIndex) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if len(created) != len(sends) {
		return nil, domain.ErrNotFound
	}
	return created, nil
}

// CompleteBatch records all provider outcomes in one statement.
func (r *QuoteSendRepository) CompleteBatch(ctx context.Context, q Querier, accountID uuid.UUID,
	outcomes []domain.QuoteSendOutcome) error {
	payload, err := json.Marshal(outcomes)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `WITH incoming AS (
		  SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(
		    "ID" uuid, "Status" text, "ProviderReference" text,
		    "SentAt" timestamptz, "ExpiresAt" timestamptz)
		)
		UPDATE quote_send send SET tracking_status = incoming."Status"::send_tracking_status,
		  provider_reference = incoming."ProviderReference", sent_at = incoming."SentAt",
		  expires_at = incoming."ExpiresAt"
		FROM incoming WHERE send.account_id = $1 AND send.id = incoming."ID"
		  AND send.tracking_status = 'PENDING'`, accountID, payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(outcomes)) {
		return domain.ErrConflict
	}
	return nil
}

// ListByQuote loads every delivery attempt for a branch-scoped quote.
func (r *QuoteSendRepository) ListByQuote(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
) ([]domain.QuoteSend, error) {
	rows, err := q.Query(ctx, `SELECT `+quoteSendColumns+`
		FROM quote_send send
		JOIN channel ON channel.account_id = send.account_id AND channel.id = send.channel_id
		JOIN quote_version version ON version.account_id = send.account_id
		  AND version.id = send.version_id
		JOIN quote ON quote.account_id = send.account_id AND quote.id = version.quote_id
		WHERE send.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		ORDER BY COALESCE(send.sent_at, send.created_at) DESC, send.created_at DESC, send.id`,
		accountID, branchID, quoteID)
	if err != nil {
		return nil, err
	}
	return scanQuoteSends(rows)
}

// ListLiveConversations returns, newest send first, the branch's SENT or CHANGE_REQUESTED quotes
// that are not archived and whose latest successful send on channelID to one of destinations has
// not expired at now.
func (r *QuoteSendRepository) ListLiveConversations(ctx context.Context, q Querier, accountID,
	branchID, channelID uuid.UUID, destinations []string, now time.Time,
) ([]domain.ConversationCandidate, error) {
	rows, err := q.Query(ctx, `SELECT latest.quote_id, latest.send_id, latest.sent_at
		FROM (
		  SELECT DISTINCT ON (version.quote_id) version.quote_id, send.id AS send_id,
		    send.sent_at, send.expires_at
		  FROM quote_send send
		  JOIN quote_version version ON version.account_id = send.account_id
		    AND version.id = send.version_id
		  WHERE send.account_id = $1 AND send.channel_id = $3 AND send.destination = ANY($4)
		    AND send.sent_at IS NOT NULL AND send.tracking_status <> 'FAILED'
		  ORDER BY version.quote_id, send.sent_at DESC, send.id DESC
		) latest
		JOIN quote ON quote.account_id = $1 AND quote.id = latest.quote_id
		WHERE quote.branch_id = $2 AND quote.archived_at IS NULL
		  AND quote.current_status IN ('SENT', 'CHANGE_REQUESTED')
		  AND latest.expires_at > $5
		ORDER BY latest.sent_at DESC, latest.send_id DESC`,
		accountID, branchID, channelID, destinations, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]domain.ConversationCandidate, 0)
	for rows.Next() {
		var candidate domain.ConversationCandidate
		if err := rows.Scan(&candidate.QuoteID, &candidate.QuoteSendID, &candidate.SentAt); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

// GetAccountIDByPublicToken performs only the scope-discovery lookup on the owner pool.
func (r *QuoteSendRepository) GetAccountIDByPublicToken(ctx context.Context, q Querier,
	token string) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := q.QueryRow(ctx, `SELECT account_id FROM quote_send
		WHERE public_token = $1 AND tracking_status IN ('SENT', 'DELIVERED', 'VIEWED')`, token).
		Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	return accountID, err
}

// GetPublicByToken verifies the token again under its resolved tenant.
func (r *QuoteSendRepository) GetPublicByToken(ctx context.Context, q Querier,
	accountID uuid.UUID, token string) (*domain.QuoteSend, error) {
	return scanQuoteSend(q.QueryRow(ctx, `SELECT `+quoteSendColumns+`
		FROM quote_send send
		JOIN channel ON channel.account_id = send.account_id AND channel.id = send.channel_id
		WHERE send.account_id = $1 AND send.public_token = $2
		  AND send.tracking_status IN ('SENT', 'DELIVERED', 'VIEWED')`, accountID, token))
}

// ListPendingEvaluations finds sent AI-generated versions without their current evaluator label.
func (r *QuoteSendRepository) ListPendingEvaluations(ctx context.Context, q Querier,
	evaluatorVersion string, limit int) ([]domain.PendingQuoteEvaluation, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT quote.account_id, quote.branch_id, quote.id, version.id
		FROM quote_send send
		JOIN quote_version version ON version.account_id = send.account_id
		  AND version.id = send.version_id AND version.is_immutable = TRUE
		JOIN quote ON quote.account_id = version.account_id AND quote.id = version.quote_id
		JOIN quote_ai_generation generation ON generation.account_id = quote.account_id
		  AND generation.quote_id = quote.id
		LEFT JOIN quote_quality_evaluation evaluation ON evaluation.account_id = quote.account_id
		  AND evaluation.generation_id = generation.id
		  AND evaluation.final_quote_version_id = version.id
		  AND evaluation.evaluator_version = $1
		WHERE send.tracking_status IN ('SENT', 'DELIVERED', 'VIEWED')
		  AND EXISTS (SELECT 1 FROM quote_status_change change
		    WHERE change.account_id = quote.account_id AND change.quote_id = quote.id
		      AND change.new_status = 'SENT')
		  AND evaluation.id IS NULL
		ORDER BY quote.id LIMIT $2`, evaluatorVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]domain.PendingQuoteEvaluation, 0)
	for rows.Next() {
		var candidate domain.PendingQuoteEvaluation
		if err := rows.Scan(&candidate.AccountID, &candidate.BranchID, &candidate.QuoteID,
			&candidate.VersionID); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func scanQuoteSends(rows pgx.Rows) ([]domain.QuoteSend, error) {
	defer rows.Close()
	sends := make([]domain.QuoteSend, 0)
	for rows.Next() {
		send, err := scanQuoteSend(rows)
		if err != nil {
			return nil, err
		}
		sends = append(sends, *send)
	}
	return sends, rows.Err()
}

func scanQuoteSend(row pgx.Row) (*domain.QuoteSend, error) {
	var send domain.QuoteSend
	err := row.Scan(&send.ID, &send.AccountID, &send.VersionID, &send.ChannelID,
		&send.ChannelType, &send.IdempotencyKey, &send.Destination, &send.ProviderReference,
		&send.PublicToken, &send.Format, &send.ValidityDays, &send.SentAt, &send.ExpiresAt,
		&send.TrackingStatus, &send.CreatedAt, &send.ChannelIdentifier, &send.ClientAssociationHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &send, nil
}
