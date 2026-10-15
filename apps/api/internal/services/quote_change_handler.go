package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

// changeOperationWriter is the line persistence that materializes a validated proposal.
type changeOperationWriter interface {
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	CreateAlternatives(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		alternatives []domain.NewQuoteItemAlternative) error
	DeleteAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	DeleteItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	UpdateItemQuantities(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		updates []domain.QuoteItemQuantityUpdate) error
	ReplaceItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		replacements []domain.QuoteItemReplacement) error
}

// applyChangeOperations writes a validated proposal onto the draft whose own lines it targets. It
// sets no price: a requantified line keeps its unit price, and a new or replaced one waits for the
// seller to accept materials.
func applyChangeOperations(ctx context.Context, q repository.Querier, lines changeOperationWriter,
	accountID, draftID uuid.UUID, proposal resolvedChangeRequest,
) error {
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
			removeIDs = append(removeIDs, *operation.TargetItemID)
			staleAlternativeIDs = append(staleAlternativeIDs, *operation.TargetItemID)
		case domain.ChangeRequestOperationUpdateQuantity:
			quantityUpdates = append(quantityUpdates, domain.QuoteItemQuantityUpdate{
				ItemID: *operation.TargetItemID, Quantity: *operation.Quantity,
			})
		case domain.ChangeRequestOperationReplaceItem:
			target := *operation.TargetItemID
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
	if err := lines.DeleteAlternativesByItemIDs(ctx, q, accountID, draftID,
		staleAlternativeIDs); err != nil {
		return err
	}
	if err := lines.DeleteItems(ctx, q, accountID, draftID, removeIDs); err != nil {
		return err
	}
	if err := lines.UpdateItemQuantities(ctx, q, accountID, draftID, quantityUpdates); err != nil {
		return err
	}
	if err := lines.ReplaceItems(ctx, q, accountID, draftID, replacements); err != nil {
		return err
	}
	if len(addedItems) > 0 {
		if _, err := lines.CreateItems(ctx, q, accountID, draftID, addedItems); err != nil {
			return err
		}
	}
	return lines.CreateAlternatives(ctx, q, accountID,
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
