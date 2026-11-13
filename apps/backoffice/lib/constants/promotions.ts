// The rule vocabulary the API accepts, shared by the server reads and the client form.
export const PROMOTION_CONDITION_TYPES = [
  'PER_ITEM',
  'QUANTITY_TIERED',
  'ITEM_SET',
  'ON_TOTAL',
] as const;
export type PromotionConditionType = (typeof PROMOTION_CONDITION_TYPES)[number];

export const PROMOTION_ACTION_TYPES = ['PERCENTAGE', 'FIXED_AMOUNT', 'SPECIAL_PRICE'] as const;
export type PromotionActionType = (typeof PROMOTION_ACTION_TYPES)[number];
