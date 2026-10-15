package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// PromotionService is the promotion surface the handler needs.
type PromotionService interface {
	List(ctx context.Context, tenant domain.Tenant) ([]domain.Promotion, error)
	Get(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Promotion, error)
	Create(ctx context.Context, tenant domain.Tenant, in domain.PromotionInput) (*domain.Promotion, error)
	Replace(ctx context.Context, tenant domain.Tenant, id uuid.UUID, in domain.PromotionInput) (*domain.Promotion, error)
}

// PromotionHandler serves the administration of an account's promotion rules.
type PromotionHandler struct {
	promotions PromotionService
}

// NewPromotionHandler builds a PromotionHandler.
func NewPromotionHandler(promotions PromotionService) *PromotionHandler {
	return &PromotionHandler{promotions: promotions}
}

// List returns every promotion of the account.
//
//	@Summary		List promotions
//	@Description	Returns every promotion rule of the account, inactive ones included, highest priority first.
//	@Tags			promotions
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.PromotionListResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/promotions [get]
func (h *PromotionHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	promotions, err := h.promotions.List(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.PromotionResponse, 0, len(promotions))
	for _, p := range promotions {
		items = append(items, toPromotionResponse(p))
	}
	c.JSON(http.StatusOK, dto.PromotionListResponse{Items: items})
}

// Get returns one promotion of the account.
//
//	@Summary		Get a promotion
//	@Description	Returns one promotion rule with its conditions and tiers.
//	@Tags			promotions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			promotionId	path		string	true	"Promotion id"
//	@Success		200			{object}	dto.PromotionResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/promotions/{promotionId} [get]
func (h *PromotionHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "promotionId")
	if !ok {
		return
	}
	promotion, err := h.promotions.Get(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toPromotionResponse(*promotion))
}

// Create adds a promotion rule to the account.
//
//	@Summary		Create a promotion
//	@Description	Adds a typed promotion rule. The engine applies it to every quote whose lines match, on its next discount sweep.
//	@Tags			promotions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.PromotionRequest	true	"Promotion"
//	@Success		201		{object}	dto.PromotionResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/promotions [post]
func (h *PromotionHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	in, ok := bindPromotion(c)
	if !ok {
		return
	}
	promotion, err := h.promotions.Create(c.Request.Context(), tenant, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toPromotionResponse(*promotion))
}

// Replace rewrites a promotion rule of the account.
//
//	@Summary		Replace a promotion
//	@Description	Rewrites the whole rule, conditions and tiers included. Deactivating or reactivating a promotion is is_active or its validity window changing here.
//	@Tags			promotions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			promotionId	path		string					true	"Promotion id"
//	@Param			request		body		dto.PromotionRequest	true	"Promotion"
//	@Success		200			{object}	dto.PromotionResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/promotions/{promotionId} [put]
func (h *PromotionHandler) Replace(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "promotionId")
	if !ok {
		return
	}
	in, ok := bindPromotion(c)
	if !ok {
		return
	}
	promotion, err := h.promotions.Replace(c.Request.Context(), tenant, id, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toPromotionResponse(*promotion))
}

// bindPromotion binds and maps a promotion body, answering the request itself when it cannot.
func bindPromotion(c *gin.Context) (domain.PromotionInput, bool) {
	var body dto.PromotionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return domain.PromotionInput{}, false
	}
	in, err := toPromotionInput(body)
	if err != nil {
		Respond(c, err)
		return domain.PromotionInput{}, false
	}
	return in, true
}

func toPromotionInput(body dto.PromotionRequest) (domain.PromotionInput, error) {
	in := domain.PromotionInput{
		BranchID: body.BranchID, Name: body.Name, Description: body.Description,
		ConditionType: domain.PromotionConditionType(body.ConditionType),
		ActionType:    domain.PromotionActionType(body.ActionType),
		ValidFrom:     body.ValidFrom, ValidTo: body.ValidTo, IsActive: true,
		IsExclusive: body.IsExclusive, Priority: body.Priority,
	}
	if body.IsActive != nil {
		in.IsActive = *body.IsActive
	}
	var err error
	if body.ActionValue != nil {
		if in.ActionValue, err = parseDecimalField(*body.ActionValue, "action_value"); err != nil {
			return in, err
		}
	}
	if in.MinTotal, err = parseOptionalDecimal(body.MinTotal, "min_total"); err != nil {
		return in, err
	}
	for i, c := range body.Conditions {
		condition := domain.PromotionCondition{ProductID: c.ProductID, FamilyID: c.FamilyID,
			SubgroupID: c.SubgroupID}
		if condition.MinQuantity, err = parseOptionalDecimal(c.MinQuantity,
			fmt.Sprintf("conditions[%d].min_quantity", i)); err != nil {
			return in, err
		}
		in.Conditions = append(in.Conditions, condition)
	}
	for i, t := range body.Tiers {
		field := fmt.Sprintf("tiers[%d]", i)
		var tier domain.PromotionTier
		if tier.FromQuantity, err = parseDecimalField(t.FromQuantity, field+".from_quantity"); err != nil {
			return in, err
		}
		if tier.ToQuantity, err = parseOptionalDecimal(t.ToQuantity, field+".to_quantity"); err != nil {
			return in, err
		}
		if tier.Value, err = parseDecimalField(t.Value, field+".value"); err != nil {
			return in, err
		}
		in.Tiers = append(in.Tiers, tier)
	}
	return in, nil
}

func parseDecimalField(raw, field string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: %s must be a valid decimal", domain.ErrInvalidInput, field)
	}
	return value, nil
}

func parseOptionalDecimal(raw *string, field string) (*decimal.Decimal, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := parseDecimalField(*raw, field)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func toPromotionResponse(p domain.Promotion) dto.PromotionResponse {
	out := dto.PromotionResponse{
		ID: p.ID, BranchID: p.BranchID, Name: p.Name, Description: p.Description,
		ConditionType: string(p.ConditionType), ActionType: string(p.ActionType),
		ActionValue: p.ActionValue.StringFixed(domain.MoneyScale),
		MinTotal:    fixedMoney(p.MinTotal), ValidFrom: p.ValidFrom, ValidTo: p.ValidTo,
		IsActive: p.IsActive, IsExclusive: p.IsExclusive, Priority: p.Priority,
		Conditions: make([]dto.PromotionConditionResponse, 0, len(p.Conditions)),
		Tiers:      make([]dto.PromotionTierResponse, 0, len(p.Tiers)),
		CreatedAt:  p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	for _, c := range p.Conditions {
		out.Conditions = append(out.Conditions, dto.PromotionConditionResponse{
			ProductID: c.ProductID, FamilyID: c.FamilyID, SubgroupID: c.SubgroupID,
			MinQuantity: fixedMoney(c.MinQuantity),
		})
	}
	for _, t := range p.Tiers {
		out.Tiers = append(out.Tiers, dto.PromotionTierResponse{
			FromQuantity: t.FromQuantity.StringFixed(domain.MoneyScale),
			ToQuantity:   fixedMoney(t.ToQuantity), Value: t.Value.StringFixed(domain.MoneyScale),
		})
	}
	return out
}

func fixedMoney(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(domain.MoneyScale)
	return &s
}
