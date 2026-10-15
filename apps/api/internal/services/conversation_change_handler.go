package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// maxHandlerText bounds the interpretation and the draft comment, both VARCHAR(512).
const maxHandlerText = 512

var (
	// errDraftMoved sends the batch back to the queue: the seller sent or rebuilt the draft, or
	// removed a line the proposal targets, while the model was answering.
	errDraftMoved = errors.New("the draft changed while the proposal was prepared")
)

type batchHandlerQuotes interface {
	quoteVersionCloneRepository
	changeOperationWriter
	GetByIDForUpdateInAccount(ctx context.Context, q repository.Querier, accountID,
		id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
		from, to domain.QuoteStatus) (*domain.Quote, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID,
		previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus,
		userID *uuid.UUID) (*domain.QuoteStatusChange, error)
	UpdateVersionTotal(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		total decimal.Decimal) (*domain.QuoteVersion, error)
}

type batchHandlerMessages interface {
	ListByBatch(ctx context.Context, q repository.Querier, accountID, quoteID,
		batchID uuid.UUID) ([]domain.QuoteMessage, error)
	ListContext(ctx context.Context, q repository.Querier, accountID, quoteID, batchID uuid.UUID,
		before time.Time, limit int) ([]domain.QuoteMessage, error)
}

type batchClaimFence interface {
	LockClaim(ctx context.Context, q repository.Querier, batch domain.MessageBatch) error
}

// BatchChangeHandlerConfig bounds what the handler sends the model and accepts back.
type BatchChangeHandlerConfig struct {
	ContextMessages int
	MaxOperations   int
}

// BatchChangeHandler applies a claimed batch of customer messages to the quote's single draft:
// the model proposes, the matrix and the draft's own lines validate, the discount sweep prices,
// and handler_decision records it for the seller.
type BatchChangeHandler struct {
	db        tenantTxRunner
	quotes    batchHandlerQuotes
	messages  batchHandlerMessages
	claims    batchClaimFence
	decisions handlerDecisionRecorder
	prices    branchPriceReader
	sweep     *discountSweep
	proposer  domain.ChangeRequestHandler
	matcher   catalogMatcher
	cfg       BatchChangeHandlerConfig
	log       *slog.Logger
}

// NewBatchChangeHandler builds a BatchChangeHandler. A nil proposer means no language model is
// configured: every batch is left to the seller as a no-match.
func NewBatchChangeHandler(db tenantTxRunner, quotes batchHandlerQuotes,
	messages batchHandlerMessages, claims batchClaimFence, decisions handlerDecisionRecorder,
	prices branchPriceReader, discounts versionDiscountStore, promotions promotionReader,
	proposer domain.ChangeRequestHandler, matcher catalogMatcher, cfg BatchChangeHandlerConfig,
	log *slog.Logger,
) *BatchChangeHandler {
	return &BatchChangeHandler{db: db, quotes: quotes, messages: messages, claims: claims,
		decisions: decisions, prices: prices,
		sweep:    &discountSweep{discounts: discounts, promotions: promotions, now: time.Now},
		proposer: proposer, matcher: matcher, cfg: cfg, log: log}
}

var _ ConversationBatchHandler = (*BatchChangeHandler)(nil)

type preparedBatch struct {
	tenant       domain.Tenant
	stateAtClaim domain.QuoteStatus
	status       domain.QuoteStatus
	draft        domain.QuoteVersion
	items        []domain.QuoteItem
	messages     []domain.QuoteMessage
	context      []domain.QuoteMessage
}

type batchProposal struct {
	record   domain.HandlerProposalRecord
	resolved *resolvedChangeRequest
}

// Handle runs one claimed batch. Its error leaves the batch for a retry, except one wrapping
// domain.ErrBatchUnprocessable, which fails it at once.
func (h *BatchChangeHandler) Handle(ctx context.Context, batch domain.MessageBatch) error {
	prepared, err := h.prepare(ctx, batch)
	if err != nil {
		return err
	}
	if len(prepared.messages) == 0 {
		return nil
	}
	proposal, err := h.propose(ctx, prepared)
	if err != nil {
		return err
	}
	return h.apply(ctx, batch, prepared, proposal)
}

// prepare moves a SENT quote to CHANGE_REQUESTED and ensures its single draft, then reads what
// the model needs. It commits before the model is called, so the move holds even if it fails.
func (h *BatchChangeHandler) prepare(ctx context.Context, batch domain.MessageBatch,
) (preparedBatch, error) {
	var prepared preparedBatch
	err := h.db.InTenantTx(ctx, domain.Tenant{AccountID: batch.AccountID},
		func(q repository.Querier) error {
			quote, err := h.lock(ctx, q, batch)
			if err != nil {
				return err
			}
			if err := domain.ValidateHandlerRun(quote.CurrentStatus, quote.ArchivedAt != nil); err != nil {
				return fmt.Errorf("%w: %w", domain.ErrBatchUnprocessable, err)
			}
			prepared.tenant = domain.Tenant{AccountID: quote.AccountID, BranchID: quote.BranchID}
			prepared.stateAtClaim = quote.CurrentStatus
			prepared.messages, err = h.messages.ListByBatch(ctx, q, batch.AccountID, quote.ID, batch.ID)
			if err != nil {
				return err
			}
			before := time.Now()
			if batch.ClosedAt != nil {
				before = *batch.ClosedAt
			}
			prepared.context, err = h.messages.ListContext(ctx, q, batch.AccountID, quote.ID, batch.ID,
				before, h.cfg.ContextMessages)
			if err != nil {
				return err
			}

			if quote.CurrentStatus == domain.QuoteStatusSent {
				previous := quote.CurrentStatus
				if _, err := h.quotes.UpdateStatus(ctx, q, quote.AccountID, quote.BranchID, quote.ID,
					previous, domain.QuoteStatusChangeRequested); err != nil {
					return err
				}
				if _, err := h.quotes.AppendStatusChange(ctx, q, quote.AccountID, quote.ID, &previous,
					domain.QuoteStatusChangeRequested, nil); err != nil {
					return err
				}
			}
			prepared.status = domain.QuoteStatusChangeRequested

			draft, err := h.quotes.GetCurrentVersion(ctx, q, quote.AccountID, quote.BranchID, quote.ID)
			if err != nil {
				return err
			}
			if draft.IsImmutable {
				var comment *string
				if text := truncateHandlerText(clientInput(prepared.messages)); text != "" {
					comment = &text
				}
				clone, err := cloneQuoteVersionForEditing(ctx, q, h.quotes, h.prices, quote.AccountID,
					quote.BranchID, quote.ID, *draft, nil, comment)
				if err != nil {
					return err
				}
				draft = clone.Draft
			}
			prepared.draft = *draft
			prepared.items, err = h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID)
			return err
		})
	return prepared, err
}

// propose asks the model about the batch and validates its answer against the draft's lines. A
// proposal the backend cannot accept becomes a no-match for the seller, not a retry.
func (h *BatchChangeHandler) propose(ctx context.Context, prepared preparedBatch,
) (batchProposal, error) {
	if h.proposer == nil {
		return noMatchProposal(domain.HandlerNoMatchAIDisabled, ""), nil
	}
	answer, err := h.proposer.Propose(domain.WithAIAccount(ctx, prepared.tenant),
		domain.ChangeRequestInput{Message: clientInput(prepared.messages),
			Context: conversationLines(prepared.context), Items: changeRequestItems(prepared.items)})
	if err != nil {
		return batchProposal{}, err
	}
	validated, err := validateChangeRequestProposal(answer, prepared.items, h.cfg.MaxOperations)
	if err != nil {
		h.log.WarnContext(ctx, "conversation handler proposal was refused",
			slog.String("draft", prepared.draft.ID.String()), slog.Any("error", err))
		interpretation := ""
		if answer != nil {
			interpretation = answer.Interpretation
		}
		return noMatchProposal(domain.HandlerNoMatchProposalRefused, interpretation), nil
	}
	if validated.Operations[0].Operation.Type == domain.ChangeRequestOperationRequiresSeller {
		return noMatchProposal(domain.HandlerNoMatchRequiresSeller,
			validated.Proposal.Interpretation), nil
	}
	if err := validateProposalIntents(prepared.status, false, validated); err != nil {
		return batchProposal{}, err
	}
	resolveChangeRequestMatches(ctx, prepared.tenant, h.matcher, &validated, h.log)
	return batchProposal{resolved: &validated, record: domain.HandlerProposalRecord{
		Outcome: domain.HandlerOutcomeApplied, Interpretation: validated.Proposal.Interpretation,
		Operations: validated.Proposal.Operations}}, nil
}

// apply re-checks the claim, the matrix and the draft under the quote's lock, then writes the
// proposal, prices the draft through the discount sweep and records the decision with the draft
// as it was before and after.
func (h *BatchChangeHandler) apply(ctx context.Context, batch domain.MessageBatch,
	prepared preparedBatch, proposal batchProposal,
) error {
	payload, err := json.Marshal(proposal.record)
	if err != nil {
		return err
	}
	return h.db.InTenantTx(ctx, prepared.tenant, func(q repository.Querier) error {
		quote, err := h.lock(ctx, q, batch)
		if err != nil {
			return err
		}
		draft, err := h.quotes.GetCurrentVersion(ctx, q, quote.AccountID, quote.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if draft.ID != prepared.draft.ID || draft.IsImmutable {
			return errDraftMoved
		}
		before, err := draftSnapshot(ctx, q, h.quotes, h.sweep.discounts, quote.AccountID, draft.ID)
		if err != nil {
			return err
		}
		after := before
		if proposal.resolved != nil {
			if err := validateProposalIntents(quote.CurrentStatus, quote.ArchivedAt != nil,
				*proposal.resolved); err != nil {
				return err
			}
			if err := h.write(ctx, q, *quote, draft.ID, *proposal.resolved); err != nil {
				return err
			}
			if after, err = draftSnapshot(ctx, q, h.quotes, h.sweep.discounts, quote.AccountID,
				draft.ID); err != nil {
				return err
			}
		}
		return h.decisions.Create(ctx, q, quote.AccountID, domain.NewHandlerDecision{
			QuoteVersionID: draft.ID, MessageBatchID: &batch.ID,
			ClientInput:      clientInput(prepared.messages),
			AIInterpretation: proposal.record.Interpretation,
			AIProposal:       string(payload), StateAtDecision: prepared.stateAtClaim,
			DraftBefore: before, DraftAfter: after,
		})
	})
}

func (h *BatchChangeHandler) write(ctx context.Context, q repository.Querier, quote domain.Quote,
	draftID uuid.UUID, proposal resolvedChangeRequest,
) error {
	current, err := h.quotes.ListItems(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return err
	}
	lines := make(map[uuid.UUID]struct{}, len(current))
	for _, item := range current {
		lines[item.ID] = struct{}{}
	}
	for _, operation := range proposal.Operations {
		if target := operation.Operation.TargetItemID; target != nil {
			if _, ok := lines[*target]; !ok {
				return errDraftMoved
			}
		}
	}
	if err := applyChangeOperations(ctx, q, h.quotes, quote.AccountID, draftID, proposal); err != nil {
		return err
	}
	items, err := h.quotes.ListItems(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return err
	}
	total, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draftID, items)
	if err != nil {
		return err
	}
	_, err = h.quotes.UpdateVersionTotal(ctx, q, quote.AccountID, draftID, total)
	return err
}

// lock takes the quote's row lock, then the batch's, the order every conversation writer follows,
// and confirms the claim is still this run's.
func (h *BatchChangeHandler) lock(ctx context.Context, q repository.Querier,
	batch domain.MessageBatch,
) (*domain.Quote, error) {
	quote, err := h.quotes.GetByIDForUpdateInAccount(ctx, q, batch.AccountID, batch.QuoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: the quote is gone", domain.ErrBatchUnprocessable)
	}
	if err != nil {
		return nil, err
	}
	if err := h.claims.LockClaim(ctx, q, batch); err != nil {
		return nil, err
	}
	return quote, nil
}

func validateProposalIntents(status domain.QuoteStatus, archived bool,
	proposal resolvedChangeRequest,
) error {
	for _, operation := range proposal.Operations {
		intent, ok := domain.HandlerIntentOf(operation.Operation.Type)
		if !ok {
			continue
		}
		if err := domain.ValidateHandlerIntent(status, archived, intent); err != nil {
			return fmt.Errorf("%w: %w", domain.ErrBatchUnprocessable, err)
		}
	}
	return nil
}

func noMatchProposal(reason domain.HandlerNoMatchReason, interpretation string) batchProposal {
	origin := domain.HandlerNoMatchDirectInput
	return batchProposal{record: domain.HandlerProposalRecord{Outcome: domain.HandlerOutcomeNoMatch,
		NoMatchOrigin: &origin, NoMatchReason: &reason,
		Interpretation: truncateHandlerText(interpretation),
		Operations:     []domain.ChangeRequestOperation{}}}
}

func clientInput(messages []domain.QuoteMessage) string {
	bodies := make([]string, 0, len(messages))
	for _, message := range messages {
		bodies = append(bodies, strings.TrimSpace(message.Body))
	}
	return strings.Join(bodies, "\n")
}

func conversationLines(messages []domain.QuoteMessage) []domain.ConversationLine {
	lines := make([]domain.ConversationLine, 0, len(messages))
	for _, message := range messages {
		lines = append(lines, domain.ConversationLine{Author: message.AuthorType, Body: message.Body})
	}
	return lines
}

// truncateHandlerText trims text and cuts it to maxHandlerText runes.
func truncateHandlerText(text string) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxHandlerText {
		text = string([]rune(text)[:maxHandlerText])
	}
	return text
}
