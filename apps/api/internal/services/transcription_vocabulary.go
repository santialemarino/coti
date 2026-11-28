package services

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// Whisper reads only the last 224 tokens of a prompt; about 800 characters of product names stays
// inside that.
const (
	vocabularyProducts = 60
	vocabularyMaxRunes = 800
)

type vocabularyRepository interface {
	ListVocabulary(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		limit int) ([]string, error)
}

// TranscriptionVocabulary lists an account's product names for a transcription to spell by.
type TranscriptionVocabulary struct {
	db       tenantTxRunner
	products vocabularyRepository
	log      *slog.Logger
}

// NewTranscriptionVocabulary builds a TranscriptionVocabulary.
func NewTranscriptionVocabulary(db tenantTxRunner, products vocabularyRepository,
	log *slog.Logger) *TranscriptionVocabulary {
	if log == nil {
		log = slog.Default()
	}
	return &TranscriptionVocabulary{db: db, products: products, log: log}
}

// For returns the account's vocabulary, or nothing when it cannot be read: a transcription without
// it is still a transcription.
func (v *TranscriptionVocabulary) For(ctx context.Context, accountID uuid.UUID) string {
	var names []string
	if err := v.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			var err error
			names, err = v.products.ListVocabulary(ctx, q, accountID, vocabularyProducts)
			return err
		}); err != nil {
		v.log.WarnContext(ctx, "transcription vocabulary unavailable", slog.Any("error", err))
		return ""
	}
	var vocabulary strings.Builder
	for _, name := range names {
		next := strings.TrimSpace(name)
		if vocabulary.Len() > 0 {
			next = ", " + next
		}
		if utf8.RuneCountInString(vocabulary.String())+utf8.RuneCountInString(next) > vocabularyMaxRunes {
			break
		}
		vocabulary.WriteString(next)
	}
	return vocabulary.String()
}
