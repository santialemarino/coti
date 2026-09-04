package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const quoteQualityEvaluationColumns = `id, account_id, generation_id,
	final_quote_version_id, evaluator_version, whole_quote_correct, same_item_count,
	all_items_equivalent, all_items_matched, all_items_priced, all_subtotals_valid,
	total_valid, created_at`

// QuoteQualityRepository reads both sides of a comparison and appends its evaluation.
type QuoteQualityRepository struct{}

// NewQuoteQualityRepository builds a QuoteQualityRepository.
func NewQuoteQualityRepository() *QuoteQualityRepository {
	return &QuoteQualityRepository{}
}

// GetGenerationByQuoteID loads the original AI proposal identity for one quote.
func (r *QuoteQualityRepository) GetGenerationByQuoteID(
	ctx context.Context, q Querier, accountID, quoteID uuid.UUID,
) (*domain.QuoteAIGeneration, error) {
	return scanQuoteAIGeneration(q.QueryRow(ctx,
		`SELECT `+quoteAIGenerationColumns+`
		 FROM quote_ai_generation
		 WHERE account_id = $1 AND quote_id = $2
		 ORDER BY created_at, id
		 LIMIT 1`, accountID, quoteID))
}

// ListGenerationItems loads the preserved proposal in client order.
func (r *QuoteQualityRepository) ListGenerationItems(
	ctx context.Context, q Querier, accountID, generationID uuid.UUID,
) ([]domain.QuoteAIGenerationItem, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, generation_id, position, source_quote_item_id, product_id,
		        requested_description, quantity, unit, quantity_source, quantity_rationale,
		        match_status, confidence_score, created_at
		 FROM quote_ai_generation_item
		 WHERE account_id = $1 AND generation_id = $2
		 ORDER BY position`, accountID, generationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.QuoteAIGenerationItem
	for rows.Next() {
		var item domain.QuoteAIGenerationItem
		if err := rows.Scan(&item.ID, &item.AccountID, &item.GenerationID, &item.Position,
			&item.SourceQuoteItemID, &item.ProductID, &item.RequestedDescription, &item.Quantity,
			&item.Unit, &item.QuantitySource, &item.QuantityRationale, &item.MatchStatus,
			&item.ConfidenceScore, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetFinalVersion loads only a frozen version with a completed send in the selected branch.
func (r *QuoteQualityRepository) GetFinalVersion(
	ctx context.Context, q Querier, accountID, branchID, quoteID, versionID uuid.UUID,
) (*domain.QuoteQualityFinalVersion, error) {
	var final domain.QuoteQualityFinalVersion
	err := q.QueryRow(ctx,
		`SELECT version.id, version.account_id, version.quote_id, version.author_id,
		        version.version_number, version.total, version.is_immutable, version.comment,
		        version.created_at,
		        COALESCE(SUM(discount.amount) FILTER (WHERE NOT discount.suppressed_by_seller), 0)
		 FROM quote
		 JOIN quote_version version ON version.account_id = $1
		                           AND version.id = $4
		                           AND version.quote_id = quote.id
		 LEFT JOIN quote_discount discount ON discount.account_id = $1
		                                  AND discount.quote_version_id = version.id
		 WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		   AND version.is_immutable = TRUE
		   AND EXISTS (
		     SELECT 1 FROM quote_send send
		     WHERE send.account_id = $1 AND send.version_id = version.id
		       AND send.sent_at IS NOT NULL
		       AND send.tracking_status IN ('SENT', 'DELIVERED', 'VIEWED')
		   )
		 GROUP BY version.id`, accountID, branchID, quoteID, versionID,
	).Scan(&final.Version.ID, &final.Version.AccountID, &final.Version.QuoteID,
		&final.Version.AuthorID, &final.Version.VersionNumber, &final.Version.Total,
		&final.Version.IsImmutable, &final.Version.Comment, &final.Version.CreatedAt,
		&final.DiscountTotal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &final, nil
}

// ListFinalItems loads the seller-approved lines from the frozen version.
func (r *QuoteQualityRepository) ListFinalItems(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	return (&QuoteRepository{}).ListItems(ctx, q, accountID, versionID)
}

// GetRawRFQText loads the original written order behind one generation.
func (r *QuoteQualityRepository) GetRawRFQText(ctx context.Context, q Querier,
	accountID, generationID uuid.UUID) (string, error) {
	var raw string
	err := q.QueryRow(ctx, `SELECT COALESCE(NULLIF(btrim(rfq.raw_text), ''), '')
	 FROM quote_ai_generation generation
	 JOIN quote ON quote.account_id = $1 AND quote.id = generation.quote_id
	 JOIN rfq ON rfq.account_id = $1 AND rfq.id = quote.rfq_id
	 WHERE generation.account_id = $1 AND generation.id = $2`, accountID, generationID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return raw, err
}

// CreateEvaluation appends one idempotent, versioned label and all of its explanations.
func (r *QuoteQualityRepository) CreateEvaluation(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewQuoteQualityEvaluation,
	differences []domain.NewQuoteQualityDifference,
) (*domain.QuoteQualityEvaluation, error) {
	evaluation, err := scanQuoteQualityEvaluation(q.QueryRow(ctx,
		`INSERT INTO quote_quality_evaluation (
		   account_id, generation_id, final_quote_version_id, evaluator_version,
		   whole_quote_correct, same_item_count, all_items_equivalent, all_items_matched,
		   all_items_priced, all_subtotals_valid, total_valid
		 )
		 SELECT $1, generation.id, version.id, $4, $5, $6, $7, $8, $9, $10, $11
		 FROM quote_ai_generation generation
		 JOIN quote_version version ON version.account_id = $1
		                           AND version.id = $3
		                           AND version.quote_id = generation.quote_id
		 WHERE generation.account_id = $1 AND generation.id = $2
		 ON CONFLICT (generation_id, final_quote_version_id, evaluator_version) DO NOTHING
		 RETURNING `+quoteQualityEvaluationColumns,
		accountID, in.GenerationID, in.FinalQuoteVersionID, in.EvaluatorVersion,
		in.WholeQuoteCorrect, in.SameItemCount, in.AllItemsEquivalent, in.AllItemsMatched,
		in.AllItemsPriced, in.AllSubtotalsValid, in.TotalValid))
	if errors.Is(err, domain.ErrNotFound) {
		return scanQuoteQualityEvaluation(q.QueryRow(ctx,
			`SELECT `+quoteQualityEvaluationColumns+`
			 FROM quote_quality_evaluation
			 WHERE account_id = $1 AND generation_id = $2 AND final_quote_version_id = $3
			   AND evaluator_version = $4`, accountID, in.GenerationID, in.FinalQuoteVersionID,
			in.EvaluatorVersion))
	}
	if err != nil {
		return nil, err
	}
	if len(differences) == 0 {
		return evaluation, nil
	}

	payload, err := json.Marshal(quoteQualityDifferencePayloads(differences))
	if err != nil {
		return nil, err
	}
	tag, err := q.Exec(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     kind text, generation_item_id uuid, final_quote_item_id uuid, field text,
		     expected_value text, actual_value text
		   )
		 )
		 INSERT INTO quote_quality_difference (
		   account_id, evaluation_id, kind, generation_item_id, final_quote_item_id,
		   field, expected_value, actual_value
		 )
		 SELECT $1, evaluation.id, incoming.kind, incoming.generation_item_id,
		        incoming.final_quote_item_id, incoming.field, incoming.expected_value,
		        incoming.actual_value
		 FROM incoming
		 JOIN quote_quality_evaluation evaluation
		   ON evaluation.account_id = $1 AND evaluation.id = $2`,
		accountID, evaluation.ID, payload)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != int64(len(differences)) {
		return nil, domain.ErrNotFound
	}
	return evaluation, nil
}

type quoteQualityDifferencePayload struct {
	Kind             string     `json:"kind"`
	GenerationItemID *uuid.UUID `json:"generation_item_id"`
	FinalQuoteItemID *uuid.UUID `json:"final_quote_item_id"`
	Field            *string    `json:"field"`
	ExpectedValue    *string    `json:"expected_value"`
	ActualValue      *string    `json:"actual_value"`
}

func quoteQualityDifferencePayloads(
	differences []domain.NewQuoteQualityDifference,
) []quoteQualityDifferencePayload {
	payloads := make([]quoteQualityDifferencePayload, 0, len(differences))
	for _, difference := range differences {
		payloads = append(payloads, quoteQualityDifferencePayload{
			Kind: string(difference.Kind), GenerationItemID: difference.GenerationItemID,
			FinalQuoteItemID: difference.FinalQuoteItemID, Field: difference.Field,
			ExpectedValue: difference.ExpectedValue, ActualValue: difference.ActualValue,
		})
	}
	return payloads
}

func scanQuoteQualityEvaluation(row pgx.Row) (*domain.QuoteQualityEvaluation, error) {
	var evaluation domain.QuoteQualityEvaluation
	err := row.Scan(&evaluation.ID, &evaluation.AccountID, &evaluation.GenerationID,
		&evaluation.FinalQuoteVersionID, &evaluation.EvaluatorVersion,
		&evaluation.WholeQuoteCorrect, &evaluation.SameItemCount,
		&evaluation.AllItemsEquivalent, &evaluation.AllItemsMatched,
		&evaluation.AllItemsPriced, &evaluation.AllSubtotalsValid, &evaluation.TotalValid,
		&evaluation.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &evaluation, nil
}
