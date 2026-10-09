package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type handlerDecisionRecorder interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewHandlerDecision) error
}

type resolvedChangeRequest struct {
	Proposal   domain.ChangeRequestProposal
	Operations []resolvedChangeOperation
}

type resolvedChangeOperation struct {
	Operation domain.ChangeRequestOperation
	Match     domain.LineMatch
}

// WithChangeRequestHandler wires the AI proposal surface used by public change requests.
func (s *QuoteDeliveryService) WithChangeRequestHandler(handler domain.ChangeRequestHandler,
	matcher catalogMatcher, decisions handlerDecisionRecorder, maxItems int, timeout time.Duration,
) *QuoteDeliveryService {
	s.changeHandler = handler
	s.matcher = matcher
	s.handlerDecisions = decisions
	s.changeMaxItems = maxItems
	s.changeTimeout = timeout
	return s
}

func (s *QuoteDeliveryService) prepareChangeRequest(ctx context.Context, accountID uuid.UUID,
	token, message string,
) *resolvedChangeRequest {
	if s.changeHandler == nil || s.changeMaxItems < 1 || s.changeTimeout <= 0 {
		return nil
	}

	var quote *domain.Quote
	var source []domain.QuoteItem
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q repository.Querier) error {
		send, err := s.sends.GetPublicByToken(ctx, q, accountID, token)
		if err != nil {
			return err
		}
		quote, err = s.quotes.GetByVersionID(ctx, q, accountID, send.VersionID)
		if err != nil {
			return err
		}
		source, err = s.quotes.ListItems(ctx, q, accountID, send.VersionID)
		return err
	})
	if err != nil {
		s.log.WarnContext(ctx, "change request proposal preparation failed", slog.Any("error", err))
		return nil
	}

	tenant := domain.Tenant{AccountID: accountID, BranchID: quote.BranchID}
	pipelineCtx, cancel := context.WithTimeout(domain.WithAIAccount(ctx, tenant), s.changeTimeout)
	defer cancel()
	proposal, err := s.changeHandler.Propose(pipelineCtx, domain.ChangeRequestInput{
		Message: message, Items: changeRequestItems(source),
	})
	if err != nil {
		s.log.WarnContext(ctx, "change request handler did not produce a proposal",
			slog.String("quote_id", quote.ID.String()), slog.Any("error", err))
		return nil
	}
	validated, err := validateChangeRequestProposal(proposal, source, s.changeMaxItems)
	if err != nil {
		s.log.WarnContext(ctx, "change request handler proposal was refused",
			slog.String("quote_id", quote.ID.String()), slog.Any("error", err))
		return nil
	}

	resolveChangeRequestMatches(pipelineCtx, tenant, s.matcher, &validated, s.log)
	return &validated
}

func changeRequestItems(items []domain.QuoteItem) []domain.ChangeRequestItem {
	result := make([]domain.ChangeRequestItem, 0, len(items))
	for _, item := range items {
		result = append(result, domain.ChangeRequestItem{ID: item.ID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity, Unit: item.Unit})
	}
	return result
}

func validateChangeRequestProposal(proposal *domain.ChangeRequestProposal, source []domain.QuoteItem,
	maxItems int,
) (resolvedChangeRequest, error) {
	if proposal == nil {
		return resolvedChangeRequest{}, fmt.Errorf("%w: change request handler returned no proposal",
			domain.ErrInvalidInput)
	}
	interpretation := strings.TrimSpace(proposal.Interpretation)
	if interpretation == "" || len([]rune(interpretation)) > 512 {
		return resolvedChangeRequest{}, fmt.Errorf("%w: change request interpretation is invalid",
			domain.ErrInvalidInput)
	}
	if len(proposal.Operations) == 0 || len(proposal.Operations) > maxItems {
		return resolvedChangeRequest{}, fmt.Errorf("%w: change request has an invalid operation count",
			domain.ErrInvalidInput)
	}

	knownItems := make(map[uuid.UUID]struct{}, len(source))
	for _, item := range source {
		knownItems[item.ID] = struct{}{}
	}
	resolved := resolvedChangeRequest{Proposal: domain.ChangeRequestProposal{
		Interpretation: interpretation,
		Operations:     make([]domain.ChangeRequestOperation, 0, len(proposal.Operations)),
	}, Operations: make([]resolvedChangeOperation, 0, len(proposal.Operations))}
	targeted := make(map[uuid.UUID]struct{}, len(proposal.Operations))
	for index, raw := range proposal.Operations {
		operation, err := validateChangeRequestOperation(raw, knownItems, targeted, index)
		if err != nil {
			return resolvedChangeRequest{}, err
		}
		if operation.Type == domain.ChangeRequestOperationRequiresSeller && len(proposal.Operations) != 1 {
			return resolvedChangeRequest{}, fmt.Errorf("%w: requires-seller cannot accompany a draft mutation",
				domain.ErrInvalidInput)
		}
		resolved.Proposal.Operations = append(resolved.Proposal.Operations, operation)
		resolved.Operations = append(resolved.Operations, resolvedChangeOperation{Operation: operation,
			Match: domain.LineMatch{MatchStatus: domain.ItemMatchStatusNoMatch}})
	}
	return resolved, nil
}

func validateChangeRequestOperation(operation domain.ChangeRequestOperation,
	knownItems, targeted map[uuid.UUID]struct{}, index int,
) (domain.ChangeRequestOperation, error) {
	operation.RequestedDescription = normalizeChangeText(operation.RequestedDescription)
	operation.Unit = normalizeChangeText(operation.Unit)
	requiresTarget := operation.Type == domain.ChangeRequestOperationRemoveItem ||
		operation.Type == domain.ChangeRequestOperationReplaceItem ||
		operation.Type == domain.ChangeRequestOperationUpdateQuantity
	requiresMaterial := operation.Type == domain.ChangeRequestOperationAddItem ||
		operation.Type == domain.ChangeRequestOperationReplaceItem
	requiresQuantity := requiresMaterial || operation.Type == domain.ChangeRequestOperationUpdateQuantity

	switch operation.Type {
	case domain.ChangeRequestOperationAddItem, domain.ChangeRequestOperationRemoveItem,
		domain.ChangeRequestOperationReplaceItem, domain.ChangeRequestOperationUpdateQuantity,
		domain.ChangeRequestOperationRequiresSeller:
	default:
		return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] has an unknown type",
			domain.ErrInvalidInput, index)
	}
	if operation.Type == domain.ChangeRequestOperationRequiresSeller {
		if operation.TargetItemID != nil || operation.RequestedDescription != nil ||
			operation.Quantity != nil || operation.Unit != nil {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: requires-seller carries a mutation",
				domain.ErrInvalidInput)
		}
		return operation, nil
	}
	if requiresTarget {
		if operation.TargetItemID == nil {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] needs a target line",
				domain.ErrInvalidInput, index)
		}
		if _, ok := knownItems[*operation.TargetItemID]; !ok {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] targets another quote",
				domain.ErrInvalidInput, index)
		}
		if _, seen := targeted[*operation.TargetItemID]; seen {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] targets a line twice",
				domain.ErrInvalidInput, index)
		}
		targeted[*operation.TargetItemID] = struct{}{}
	} else if operation.TargetItemID != nil {
		return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] must not target a line",
			domain.ErrInvalidInput, index)
	}
	if requiresMaterial {
		if operation.RequestedDescription == nil || len([]rune(*operation.RequestedDescription)) > 512 {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] needs a material description",
				domain.ErrInvalidInput, index)
		}
		if operation.Unit != nil && len([]rune(*operation.Unit)) > 64 {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] unit is too long",
				domain.ErrInvalidInput, index)
		}
	} else if operation.RequestedDescription != nil || operation.Unit != nil {
		return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] carries an unexpected material",
			domain.ErrInvalidInput, index)
	}
	if requiresQuantity {
		if operation.Quantity == nil || !operation.Quantity.IsPositive() {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] needs a positive quantity",
				domain.ErrInvalidInput, index)
		}
		if err := validateAmount(*operation.Quantity, fmt.Sprintf("operations[%d].quantity", index)); err != nil {
			return domain.ChangeRequestOperation{}, err
		}
	} else if operation.Quantity != nil {
		return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d] carries an unexpected quantity",
			domain.ErrInvalidInput, index)
	}
	return operation, nil
}

func normalizeChangeText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func resolveChangeRequestMatches(ctx context.Context, tenant domain.Tenant, matcher catalogMatcher,
	resolved *resolvedChangeRequest, log *slog.Logger,
) {
	if matcher == nil {
		return
	}
	descriptions := make([]string, 0, len(resolved.Operations))
	indexes := make([]int, 0, len(resolved.Operations))
	for index, operation := range resolved.Operations {
		if operation.Operation.Type != domain.ChangeRequestOperationAddItem &&
			operation.Operation.Type != domain.ChangeRequestOperationReplaceItem {
			continue
		}
		descriptions = append(descriptions, *operation.Operation.RequestedDescription)
		indexes = append(indexes, index)
	}
	if len(descriptions) == 0 {
		return
	}
	matches, err := matcher.Match(ctx, tenant, descriptions)
	if err != nil || len(matches) != len(descriptions) {
		if err != nil {
			log.WarnContext(ctx, "change request catalog matching did not run", slog.Any("error", err),
				slog.Int("lines", len(descriptions)))
		} else {
			log.ErrorContext(ctx, "change request catalog matching returned a different number of decisions",
				slog.Int("decisions", len(matches)), slog.Int("lines", len(descriptions)))
		}
		return
	}
	for i, index := range indexes {
		resolved.Operations[index].Match = matches[i]
	}
}

func (s *QuoteDeliveryService) applyChangeRequestProposal(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, clone *quoteVersionClone, proposal *resolvedChangeRequest,
) error {
	if proposal == nil || len(proposal.Operations) == 0 ||
		proposal.Operations[0].Operation.Type == domain.ChangeRequestOperationRequiresSeller {
		return nil
	}

	removeIDs := make([]uuid.UUID, 0)
	quantityUpdates := make([]domain.QuoteItemQuantityUpdate, 0)
	replacements := make([]domain.QuoteItemReplacement, 0)
	replacementAlternatives := make([]domain.NewQuoteItemAlternative, 0)
	addedItems := make([]domain.NewQuoteItem, 0)
	addedAlternatives := make([]domain.NewQuoteItemAlternative, 0)
	staleAlternativeIDs := make([]uuid.UUID, 0)

	for _, resolved := range proposal.Operations {
		operation := resolved.Operation
		switch operation.Type {
		case domain.ChangeRequestOperationAddItem:
			item := newChangeRequestItem(operation, resolved.Match)
			addedItems = append(addedItems, item)
			addedAlternatives = append(addedAlternatives, alternativesFromMatch(item.ID, resolved.Match)...)
		case domain.ChangeRequestOperationRemoveItem:
			target, err := clonedTargetID(clone, *operation.TargetItemID)
			if err != nil {
				return err
			}
			removeIDs = append(removeIDs, target)
			staleAlternativeIDs = append(staleAlternativeIDs, target)
		case domain.ChangeRequestOperationUpdateQuantity:
			target, err := clonedTargetID(clone, *operation.TargetItemID)
			if err != nil {
				return err
			}
			quantityUpdates = append(quantityUpdates, domain.QuoteItemQuantityUpdate{
				ItemID: target, Quantity: *operation.Quantity,
			})
		case domain.ChangeRequestOperationReplaceItem:
			target, err := clonedTargetID(clone, *operation.TargetItemID)
			if err != nil {
				return err
			}
			replacements = append(replacements, domain.QuoteItemReplacement{ItemID: target,
				ProductID: resolved.Match.ProductID, RequestedDescription: *operation.RequestedDescription,
				Quantity: *operation.Quantity, Unit: operation.Unit,
				ConfidenceScore: decimal.NewNullDecimal(resolved.Match.Confidence),
				MatchStatus:     resolved.Match.MatchStatus})
			replacementAlternatives = append(replacementAlternatives,
				alternativesFromMatch(target, resolved.Match)...)
			staleAlternativeIDs = append(staleAlternativeIDs, target)
		}
	}
	if err := s.quotes.DeleteAlternativesByItemIDs(ctx, q, accountID, clone.Draft.ID,
		staleAlternativeIDs); err != nil {
		return err
	}
	if err := s.quotes.DeleteItems(ctx, q, accountID, clone.Draft.ID, removeIDs); err != nil {
		return err
	}
	if err := s.quotes.UpdateItemQuantities(ctx, q, accountID, clone.Draft.ID, quantityUpdates); err != nil {
		return err
	}
	if err := s.quotes.ReplaceItems(ctx, q, accountID, clone.Draft.ID, replacements); err != nil {
		return err
	}
	if len(addedItems) > 0 {
		if _, err := s.quotes.CreateItems(ctx, q, accountID, clone.Draft.ID, addedItems); err != nil {
			return err
		}
	}
	return s.quotes.CreateAlternatives(ctx, q, accountID,
		append(replacementAlternatives, addedAlternatives...))
}

func newChangeRequestItem(operation domain.ChangeRequestOperation,
	match domain.LineMatch,
) domain.NewQuoteItem {
	return domain.NewQuoteItem{ID: uuid.New(), ProductID: match.ProductID,
		RequestedDescription: *operation.RequestedDescription, Quantity: *operation.Quantity,
		Unit: operation.Unit, ConfidenceScore: decimal.NewNullDecimal(match.Confidence),
		MatchStatus: match.MatchStatus}
}

func clonedTargetID(clone *quoteVersionClone, sourceID uuid.UUID) (uuid.UUID, error) {
	target, ok := clone.SourceItemIDs[sourceID]
	if !ok {
		return uuid.Nil, fmt.Errorf("%w: change request target did not exist in the sent version",
			domain.ErrInvalidInput)
	}
	return target, nil
}

func (s *QuoteDeliveryService) recordChangeRequestProposal(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, draftID uuid.UUID, message string, proposal *resolvedChangeRequest,
) error {
	if proposal == nil || s.handlerDecisions == nil {
		return nil
	}
	payload, err := json.Marshal(proposal.Proposal)
	if err != nil {
		return err
	}
	return s.handlerDecisions.Create(ctx, q, accountID, domain.NewHandlerDecision{
		QuoteVersionID: draftID, ClientInput: message, AIInterpretation: proposal.Proposal.Interpretation,
		AIProposal: string(payload), StateAtDecision: domain.QuoteStatusSent,
	})
}
