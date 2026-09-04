package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const quoteAIGenerationColumns = `id, account_id, quote_id, quote_version_id, provider, model,
	prompt_version, schema_version, input_tokens, output_tokens, cache_read_tokens,
	cache_write_tokens, created_at`

const quoteAIGenerationVersionIndex = "uq_quote_ai_generation_version"

// QuoteAIGenerationRepository owns the immutable copy of each AI-proposed quote draft.
type QuoteAIGenerationRepository struct{}

// NewQuoteAIGenerationRepository builds a QuoteAIGenerationRepository.
func NewQuoteAIGenerationRepository() *QuoteAIGenerationRepository {
	return &QuoteAIGenerationRepository{}
}

// Create writes a generation and its proposed lines without relying on the editable quote rows.
func (r *QuoteAIGenerationRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewQuoteAIGeneration,
	items []domain.NewQuoteAIGenerationItem,
) (*domain.QuoteAIGeneration, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: an AI quote generation must carry at least one item",
			domain.ErrInvalidInput)
	}
	if in.Provider == "" || in.Model == "" || in.PromptVersion == "" || in.SchemaVersion == "" {
		return nil, fmt.Errorf("%w: an AI quote generation must carry reproducible metadata",
			domain.ErrInvalidInput)
	}

	generation, err := scanQuoteAIGeneration(q.QueryRow(ctx,
		`INSERT INTO quote_ai_generation (
		   account_id, quote_id, quote_version_id, provider, model, prompt_version,
		   schema_version, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens
		 )
		 SELECT $1, quote.id, version.id, $4, $5, $6, $7, $8, $9, $10, $11
		 FROM quote
		 JOIN quote_version version ON version.account_id = $1
		                           AND version.id = $3
		                           AND version.quote_id = quote.id
		 WHERE quote.account_id = $1 AND quote.id = $2
		 RETURNING `+quoteAIGenerationColumns,
		accountID, in.QuoteID, in.QuoteVersionID, in.Provider, in.Model, in.PromptVersion,
		in.SchemaVersion, in.InputTokens, in.OutputTokens, in.CacheReadTokens,
		in.CacheWriteTokens))
	if isUniqueViolation(err, quoteAIGenerationVersionIndex) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(quoteAIGenerationItemPayloads(items))
	if err != nil {
		return nil, err
	}
	tag, err := q.Exec(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     position int, source_quote_item_id uuid, product_id uuid,
		     requested_description text, quantity numeric,
		     unit text, quantity_source text, quantity_rationale text, match_status text,
		     confidence_score numeric
		   )
		 )
		 INSERT INTO quote_ai_generation_item (
		   account_id, generation_id, position, source_quote_item_id, product_id,
		   requested_description, quantity, unit, quantity_source, quantity_rationale,
		   match_status, confidence_score
		 )
		 SELECT $1, generation.id, incoming.position, incoming.source_quote_item_id,
		        incoming.product_id, incoming.requested_description, incoming.quantity, incoming.unit,
		        incoming.quantity_source, incoming.quantity_rationale,
		        incoming.match_status::item_match_status, incoming.confidence_score
		 FROM incoming
		 JOIN quote_ai_generation generation ON generation.account_id = $1 AND generation.id = $2
		 LEFT JOIN product ON product.account_id = $1 AND product.id = incoming.product_id
		 WHERE incoming.product_id IS NULL OR product.id IS NOT NULL`,
		accountID, generation.ID, payload)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != int64(len(items)) {
		return nil, domain.ErrNotFound
	}
	return generation, nil
}

type quoteAIGenerationItemPayload struct {
	Position             int        `json:"position"`
	SourceQuoteItemID    uuid.UUID  `json:"source_quote_item_id"`
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription string     `json:"requested_description"`
	Quantity             string     `json:"quantity"`
	Unit                 *string    `json:"unit"`
	QuantitySource       string     `json:"quantity_source"`
	QuantityRationale    string     `json:"quantity_rationale"`
	MatchStatus          string     `json:"match_status"`
	ConfidenceScore      *string    `json:"confidence_score"`
}

func quoteAIGenerationItemPayloads(
	items []domain.NewQuoteAIGenerationItem,
) []quoteAIGenerationItemPayload {
	payloads := make([]quoteAIGenerationItemPayload, 0, len(items))
	for _, item := range items {
		payloads = append(payloads, quoteAIGenerationItemPayload{
			Position: item.Position, SourceQuoteItemID: item.SourceQuoteItemID,
			ProductID:            item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity.String(),
			Unit: item.Unit, QuantitySource: string(item.QuantitySource),
			QuantityRationale: item.QuantityRationale, MatchStatus: string(item.MatchStatus),
			ConfidenceScore: nullDecimalString(item.ConfidenceScore),
		})
	}
	return payloads
}

func scanQuoteAIGeneration(row pgx.Row) (*domain.QuoteAIGeneration, error) {
	var generation domain.QuoteAIGeneration
	err := row.Scan(&generation.ID, &generation.AccountID, &generation.QuoteID,
		&generation.QuoteVersionID, &generation.Provider, &generation.Model,
		&generation.PromptVersion, &generation.SchemaVersion, &generation.InputTokens,
		&generation.OutputTokens, &generation.CacheReadTokens, &generation.CacheWriteTokens,
		&generation.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &generation, nil
}
