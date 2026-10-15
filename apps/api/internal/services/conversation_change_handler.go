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
	ApplyPricing(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		pricings []domain.QuoteItemPricing) error
}

// batchHandlerDiscounts is the discount persistence the sweep needs, plus restoring a suppression.
type batchHandlerDiscounts interface {
	versionDiscountStore
	draftDiscountCopier
	UpdateByID(ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID,
		in domain.QuoteDiscountUpdate) (*domain.QuoteDiscount, error)
}

// batchHandlerAlternatives reads the catalog's alternatives a complex intent's recipe swaps in.
type batchHandlerAlternatives interface {
	ListByBaseProductIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		baseProductIDs []uuid.UUID, kind domain.ProductAlternativeType,
	) (map[uuid.UUID][]domain.ProductAlternativeView, error)
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
// the model proposes, the matrix and the draft's own lines validate, a complex intent runs its
// recipe, the discount sweep prices, and handler_decision records it for the seller.
type BatchChangeHandler struct {
	db           tenantTxRunner
	quotes       batchHandlerQuotes
	messages     batchHandlerMessages
	claims       batchClaimFence
	decisions    handlerDecisionRecorder
	prices       branchPriceReader
	discounts    batchHandlerDiscounts
	alternatives batchHandlerAlternatives
	sweep        *discountSweep
	proposer     domain.ChangeRequestHandler
	matcher      catalogMatcher
	cfg          BatchChangeHandlerConfig
	log          *slog.Logger
}

// NewBatchChangeHandler builds a BatchChangeHandler. A nil proposer means no language model is
// configured: every batch is left to the seller as a no-match.
func NewBatchChangeHandler(db tenantTxRunner, quotes batchHandlerQuotes,
	messages batchHandlerMessages, claims batchClaimFence, decisions handlerDecisionRecorder,
	prices branchPriceReader, discounts batchHandlerDiscounts, promotions promotionReader,
	alternatives batchHandlerAlternatives, proposer domain.ChangeRequestHandler,
	matcher catalogMatcher, cfg BatchChangeHandlerConfig, log *slog.Logger,
) *BatchChangeHandler {
	return &BatchChangeHandler{db: db, quotes: quotes, messages: messages, claims: claims,
		decisions: decisions, prices: prices, discounts: discounts, alternatives: alternatives,
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
	promotions   []domain.Promotion
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
	err = h.apply(ctx, batch, prepared, proposal)
	var exhausted *recipeNoMatch
	if errors.As(err, &exhausted) {
		return h.apply(ctx, batch, prepared, exhausted.proposal(proposal.record))
	}
	return err
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
					quote.BranchID, quote.ID, *draft, nil, comment,
					&draftCarry{discounts: h.discounts, sweep: h.sweep, totals: h.quotes})
				if err != nil {
					return err
				}
				draft = clone.Draft
			}
			prepared.draft = *draft
			prepared.items, err = h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID)
			if err != nil || h.sweep.promotions == nil {
				return err
			}
			prepared.promotions, err = h.sweep.promotions.ListInForce(ctx, q, quote.AccountID,
				quote.BranchID, h.sweep.now())
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
			Context: conversationLines(prepared.context), Items: changeRequestItems(prepared.items),
			Promotions: changeRequestPromotions(prepared.promotions)})
	if err != nil {
		return batchProposal{}, err
	}
	validated, err := validateChangeRequestProposal(answer, prepared.items, prepared.promotions,
		h.cfg.MaxOperations)
	if err != nil {
		h.log.WarnContext(ctx, "conversation handler proposal was refused",
			slog.String("draft", prepared.draft.ID.String()), slog.Any("error", err))
		interpretation := ""
		if answer != nil {
			interpretation = answer.Interpretation
		}
		return noMatchProposal(domain.HandlerNoMatchProposalRefused, interpretation), nil
	}
	if validated.Proposal.Level == domain.ChangeRequestLevelNoMatch {
		return noMatchProposal(domain.HandlerNoMatchRequiresSeller,
			validated.Proposal.Interpretation), nil
	}
	if err := validateProposalIntents(prepared.status, false, validated); err != nil {
		return batchProposal{}, err
	}
	resolveChangeRequestMatches(ctx, prepared.tenant, h.matcher, &validated, h.log)
	return batchProposal{resolved: &validated, record: domain.HandlerProposalRecord{
		Outcome: domain.HandlerOutcomeApplied, Interpretation: validated.Proposal.Interpretation,
		Intent: validated.Proposal.Intent, IntentItemIDs: validated.Proposal.IntentItemIDs,
		Operations: validated.Proposal.Operations}}, nil
}

// apply re-checks the claim, the matrix and the draft under the quote's lock, then writes the
// proposal, prices the draft through the discount sweep and records the decision with the draft
// as it was before and after. A recipe or a promotion that cannot deliver returns a recipeNoMatch
// and rolls the whole write back.
func (h *BatchChangeHandler) apply(ctx context.Context, batch domain.MessageBatch,
	prepared preparedBatch, proposal batchProposal,
) error {
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
		record, after := proposal.record, before
		if proposal.resolved != nil {
			if err := validateProposalIntents(quote.CurrentStatus, quote.ArchivedAt != nil,
				*proposal.resolved); err != nil {
				return err
			}
			written, err := h.write(ctx, q, *quote, *draft, *proposal.resolved)
			if err != nil {
				return err
			}
			record.Operations, record.StrategiesTried = written.operations, written.strategies
			if after, err = draftSnapshot(ctx, q, h.quotes, h.sweep.discounts, quote.AccountID,
				draft.ID); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return h.decisions.Create(ctx, q, quote.AccountID, domain.NewHandlerDecision{
			QuoteVersionID: draft.ID, MessageBatchID: &batch.ID,
			ClientInput:      clientInput(prepared.messages),
			AIInterpretation: record.Interpretation,
			AIProposal:       string(payload), StateAtDecision: prepared.stateAtClaim,
			DraftBefore: before, DraftAfter: after,
		})
	})
}

// handlerWrite is what a proposal turned into on the draft.
type handlerWrite struct {
	operations []domain.ChangeRequestOperation
	strategies []domain.HandlerStrategy
}

func (h *BatchChangeHandler) write(ctx context.Context, q repository.Querier, quote domain.Quote,
	draft domain.QuoteVersion, proposal resolvedChangeRequest,
) (handlerWrite, error) {
	current, err := h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID)
	if err != nil {
		return handlerWrite{}, err
	}
	lines := make(map[uuid.UUID]struct{}, len(current))
	for _, item := range current {
		lines[item.ID] = struct{}{}
	}
	targets := proposal.Proposal.IntentItemIDs
	for _, operation := range proposal.Operations {
		if target := operation.Operation.TargetItemID; target != nil {
			targets = append(targets, *target)
		}
	}
	for _, target := range targets {
		if _, ok := lines[target]; !ok {
			return handlerWrite{}, errDraftMoved
		}
	}
	if proposal.Proposal.Level == domain.ChangeRequestLevelComplex {
		return h.runRecipe(ctx, q, quote, draft, current, proposal.Proposal)
	}

	if err := applyChangeOperations(ctx, q, h.quotes, quote.AccountID, draft.ID, proposal); err != nil {
		return handlerWrite{}, err
	}
	items, err := h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID)
	if err != nil {
		return handlerWrite{}, err
	}
	total, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draft.ID, items)
	if err != nil {
		return handlerWrite{}, err
	}
	requested := make([]uuid.UUID, 0)
	for _, operation := range proposal.Operations {
		if operation.Operation.Type == domain.ChangeRequestOperationAddDiscount {
			requested = append(requested, *operation.Operation.PromotionID)
		}
	}
	if len(requested) > 0 {
		if total, err = h.restorePromotions(ctx, q, quote, draft.ID, items, requested,
			total); err != nil {
			return handlerWrite{}, err
		}
	}
	if _, err := h.quotes.UpdateVersionTotal(ctx, q, quote.AccountID, draft.ID, total); err != nil {
		return handlerWrite{}, err
	}
	return handlerWrite{operations: proposal.Proposal.Operations}, nil
}

// restorePromotions makes each requested promotion live on the draft: one the sweep already
// earned needs nothing, one the seller suppressed is restored and re-earned, and one the quote
// does not earn is a no-match. Returns the total after any restore.
func (h *BatchChangeHandler) restorePromotions(ctx context.Context, q repository.Querier,
	quote domain.Quote, draftID uuid.UUID, items []domain.QuoteItem, requested []uuid.UUID,
	total decimal.Decimal,
) (decimal.Decimal, error) {
	discounts, err := h.discounts.ListByVersionID(ctx, q, quote.AccountID, draftID)
	if err != nil {
		return decimal.Zero, err
	}
	restore := make([]uuid.UUID, 0)
	for _, promotionID := range requested {
		if promotionLive(discounts, promotionID) {
			continue
		}
		suppressed := false
		for _, discount := range discounts {
			if discount.Origin == domain.DiscountOriginAutomatic && discount.SuppressedBySeller &&
				discount.PromotionID != nil && *discount.PromotionID == promotionID {
				restore = append(restore, discount.ID)
				suppressed = true
			}
		}
		if !suppressed {
			return decimal.Zero, &recipeNoMatch{reason: domain.HandlerNoMatchPromotionNotEarned}
		}
	}
	if len(restore) == 0 {
		return total, nil
	}
	unsuppressed := false
	for _, discountID := range restore {
		if _, err := h.discounts.UpdateByID(ctx, q, quote.AccountID, draftID, discountID,
			domain.QuoteDiscountUpdate{SuppressedBySeller: &unsuppressed}); err != nil {
			return decimal.Zero, err
		}
	}
	if total, err = h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draftID,
		items); err != nil {
		return decimal.Zero, err
	}
	if discounts, err = h.discounts.ListByVersionID(ctx, q, quote.AccountID, draftID); err != nil {
		return decimal.Zero, err
	}
	for _, promotionID := range requested {
		if !promotionLive(discounts, promotionID) {
			return decimal.Zero, &recipeNoMatch{reason: domain.HandlerNoMatchPromotionNotEarned}
		}
	}
	return total, nil
}

// runRecipe reaches a complex intent by swapping the targeted lines for catalog alternatives:
// the cheapest priced ECONOMY one below the line's price to lower the total, the cheapest priced
// PREMIUM one to improve quality. Quantities never change. A lower-price result the engine does
// not end below the draft's own total is dropped.
func (h *BatchChangeHandler) runRecipe(ctx context.Context, q repository.Querier,
	quote domain.Quote, draft domain.QuoteVersion, current []domain.QuoteItem,
	proposal domain.ChangeRequestProposal,
) (handlerWrite, error) {
	strategy, kind := domain.HandlerStrategyImproveQuality, domain.ProductAlternativePremium
	if *proposal.Intent == domain.ChangeRequestIntentLowerTotalPrice {
		strategy, kind = domain.HandlerStrategyFindCheaperAlternatives,
			domain.ProductAlternativeEconomy
	}
	exhausted := &recipeNoMatch{reason: domain.HandlerNoMatchRecipesExhausted,
		strategies: []domain.HandlerStrategy{strategy}}

	named := make(map[uuid.UUID]struct{}, len(proposal.IntentItemIDs))
	for _, id := range proposal.IntentItemIDs {
		named[id] = struct{}{}
	}
	targets := make([]domain.QuoteItem, 0, len(current))
	for _, item := range current {
		if _, ok := named[item.ID]; item.ProductID != nil && (len(named) == 0 || ok) {
			targets = append(targets, item)
		}
	}
	// The bar is the engine's total for the draft as it stands, not a total stored before a sweep.
	baseline, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draft.ID, current)
	if err != nil {
		return handlerWrite{}, err
	}
	alternatives, err := h.alternatives.ListByBaseProductIDs(ctx, q, quote.AccountID,
		quoteItemProductIDs(targets), kind)
	if err != nil {
		return handlerWrite{}, err
	}
	candidateIDs := make([]uuid.UUID, 0)
	for _, offered := range alternatives {
		for _, alternative := range offered {
			candidateIDs = append(candidateIDs, alternative.Product.ID)
		}
	}
	prices, err := h.prices.GetCurrentByProductIDs(ctx, q, quote.AccountID, quote.BranchID,
		candidateIDs)
	if err != nil {
		return handlerWrite{}, err
	}

	resolved := resolvedChangeRequest{Proposal: proposal}
	replaced := make(map[uuid.UUID]struct{}, len(targets))
	for _, target := range targets {
		best, ok := pickAlternative(alternatives[*target.ProductID], prices, target, strategy)
		if !ok {
			continue
		}
		id, description, quantity := target.ID, best.Product.CanonicalName, target.Quantity
		operation := domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationReplaceItem,
			TargetItemID: &id, RequestedDescription: &description, Quantity: &quantity,
			Unit: target.Unit}
		productID := best.Product.ID
		resolved.Operations = append(resolved.Operations, resolvedChangeOperation{
			Operation: operation, Match: domain.LineMatch{ProductID: &productID,
				MatchStatus: domain.ItemMatchStatusMatched, Confidence: decimal.NewFromInt(1)}})
		resolved.Proposal.Operations = append(resolved.Proposal.Operations, operation)
		replaced[id] = struct{}{}
	}
	if len(resolved.Operations) == 0 {
		return handlerWrite{}, exhausted
	}
	if err := applyChangeOperations(ctx, q, h.quotes, quote.AccountID, draft.ID, resolved); err != nil {
		return handlerWrite{}, err
	}

	// A replacement is written unpriced; the recipe chose it by its branch price, so it freezes it.
	items, err := h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID)
	if err != nil {
		return handlerWrite{}, err
	}
	swapped := make([]domain.QuoteItem, 0, len(replaced))
	for _, item := range items {
		if _, ok := replaced[item.ID]; ok {
			swapped = append(swapped, item)
		}
	}
	valuation, err := valueQuoteItems(swapped, prices)
	if err != nil {
		return handlerWrite{}, err
	}
	if err := h.quotes.ApplyPricing(ctx, q, quote.AccountID, draft.ID, valuation.pricings); err != nil {
		return handlerWrite{}, err
	}
	if items, err = h.quotes.ListItems(ctx, q, quote.AccountID, draft.ID); err != nil {
		return handlerWrite{}, err
	}
	total, err := h.sweep.settle(ctx, q, quote.AccountID, quote.BranchID, draft.ID, items)
	if err != nil {
		return handlerWrite{}, err
	}
	if strategy == domain.HandlerStrategyFindCheaperAlternatives && !total.LessThan(baseline) {
		return handlerWrite{}, exhausted
	}
	if _, err := h.quotes.UpdateVersionTotal(ctx, q, quote.AccountID, draft.ID, total); err != nil {
		return handlerWrite{}, err
	}
	return handlerWrite{operations: resolved.Proposal.Operations,
		strategies: []domain.HandlerStrategy{strategy}}, nil
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
	if proposal.Proposal.Level == domain.ChangeRequestLevelComplex {
		if err := domain.ValidateHandlerIntent(status, archived, domain.HandlerIntentComplex); err != nil {
			return fmt.Errorf("%w: %w", domain.ErrBatchUnprocessable, err)
		}
	}
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

// recipeNoMatch rolls a run back so it is recorded as a no-match: its goal was understood, but no
// recipe or promotion could deliver it.
type recipeNoMatch struct {
	reason     domain.HandlerNoMatchReason
	strategies []domain.HandlerStrategy
}

func (e *recipeNoMatch) Error() string {
	return "the handler could not deliver the request: " + string(e.reason)
}

// proposal is the no-match the run records instead, keeping what the model understood.
func (e *recipeNoMatch) proposal(understood domain.HandlerProposalRecord) batchProposal {
	origin := domain.HandlerNoMatchPipelineExhausted
	reason := e.reason
	return batchProposal{record: domain.HandlerProposalRecord{Outcome: domain.HandlerOutcomeNoMatch,
		NoMatchOrigin: &origin, NoMatchReason: &reason, Interpretation: understood.Interpretation,
		Intent: understood.Intent, IntentItemIDs: understood.IntentItemIDs,
		StrategiesTried: e.strategies,
		Operations:      []domain.ChangeRequestOperation{}}}
}

// pickAlternative returns the cheapest alternative the branch prices, ties broken by name. A
// cheaper-alternatives recipe takes only one priced below the line's own unit price.
func pickAlternative(offered []domain.ProductAlternativeView, prices map[uuid.UUID]domain.BranchPrice,
	line domain.QuoteItem, strategy domain.HandlerStrategy,
) (domain.ProductAlternativeView, bool) {
	var best domain.ProductAlternativeView
	var bestPrice decimal.Decimal
	found := false
	for _, alternative := range offered {
		price, ok := prices[alternative.Product.ID]
		if !ok {
			continue
		}
		if strategy == domain.HandlerStrategyFindCheaperAlternatives &&
			(!line.UnitPriceSnapshot.Valid || !price.Price.LessThan(line.UnitPriceSnapshot.Decimal)) {
			continue
		}
		if !found || price.Price.LessThan(bestPrice) || (price.Price.Equal(bestPrice) &&
			alternative.Product.CanonicalName < best.Product.CanonicalName) {
			best, bestPrice, found = alternative, price.Price, true
		}
	}
	return best, found
}

func promotionLive(discounts []domain.QuoteDiscount, promotionID uuid.UUID) bool {
	for _, discount := range discounts {
		if !discount.SuppressedBySeller && discount.PromotionID != nil &&
			*discount.PromotionID == promotionID {
			return true
		}
	}
	return false
}

func changeRequestPromotions(promotions []domain.Promotion) []domain.ChangeRequestPromotion {
	result := make([]domain.ChangeRequestPromotion, 0, len(promotions))
	for _, promotion := range promotions {
		result = append(result, domain.ChangeRequestPromotion{ID: promotion.ID, Name: promotion.Name,
			Description: promotion.Description})
	}
	return result
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
