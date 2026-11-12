import { z } from 'zod';

import {
  PROMOTION_ACTION_TYPES,
  PROMOTION_CONDITION_TYPES,
  type PromotionConditionType,
} from '@/lib/constants/promotions';
import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

// The API's own caps for a rule's name and description.
export const PROMOTION_NAME_MAX_LENGTH = 128;
export const PROMOTION_DESCRIPTION_MAX_LENGTH = 512;
const PERCENTAGE_MAX = 100;

export const CONDITION_TARGET_KINDS = ['product', 'family', 'subgroup'] as const;
export type ConditionTargetKind = (typeof CONDITION_TARGET_KINDS)[number];

// How many targets each condition type needs; the API refuses anything else.
export const MIN_CONDITIONS: Record<PromotionConditionType, number> = {
  PER_ITEM: 1,
  QUANTITY_TIERED: 1,
  ITEM_SET: 2,
  ON_TOTAL: 0,
};

// A special price replaces a unit price, so it only makes sense where the rule targets units.
export function allowsSpecialPrice(conditionType: PromotionConditionType): boolean {
  return conditionType === 'PER_ITEM' || conditionType === 'QUANTITY_TIERED';
}

const isPositive = (raw: string) => raw !== '' && Number(raw) > 0;

/*
 * Figures stay the canonical decimal strings `AmountInput` holds, and the cross-field rules — which
 * depend on the condition type — are checked once over the whole object, each reported on the field
 * the caller has to change.
 */
export function promotionSchema(t: SchemaText = rawText) {
  const condition = z.object({
    kind: z.enum(CONDITION_TARGET_KINDS),
    targetId: z.string(),
    // The target's name, for display only; the API takes the id.
    label: z.string(),
    minQuantity: z.string(),
  });
  const tier = z.object({ fromQuantity: z.string(), toQuantity: z.string(), value: z.string() });

  return z
    .object({
      name: requiredText(t, 'name.required', PROMOTION_NAME_MAX_LENGTH),
      description: optionalText(t, PROMOTION_DESCRIPTION_MAX_LENGTH),
      // Empty means every branch of the account.
      branchId: z.string(),
      conditionType: z.enum(PROMOTION_CONDITION_TYPES),
      actionType: z.enum(PROMOTION_ACTION_TYPES),
      actionValue: z.string(),
      minTotal: z.string(),
      // Calendar days (YYYY-MM-DD), or empty for an open end.
      validFrom: z.string(),
      validTo: z.string(),
      isActive: z.boolean(),
      isExclusive: z.boolean(),
      priority: z.string().refine((raw) => /^\d+$/.test(raw), t.field('priority.required')),
      conditions: z.array(condition),
      tiers: z.array(tier),
    })
    .superRefine((values, ctx) => {
      const issue = (path: (string | number)[], key: string, params?: Record<string, number>) =>
        ctx.addIssue({ code: 'custom', path, message: t.field(key, params) });
      const tiered = values.conditionType === 'QUANTITY_TIERED';
      const percentage = values.actionType === 'PERCENTAGE';

      if (values.actionType === 'SPECIAL_PRICE' && !allowsSpecialPrice(values.conditionType)) {
        issue(['actionType'], 'actionType.specialPriceScope');
      }

      if (!tiered) {
        if (!isPositive(values.actionValue)) issue(['actionValue'], 'actionValue.required');
        else if (percentage && Number(values.actionValue) > PERCENTAGE_MAX) {
          issue(['actionValue'], 'actionValue.percentageMax', { max: PERCENTAGE_MAX });
        }
      }

      if (values.conditionType === 'ON_TOTAL' && values.minTotal !== '') {
        if (!isPositive(values.minTotal)) issue(['minTotal'], 'minTotal.positive');
      }

      if (values.validFrom && values.validTo && values.validTo < values.validFrom) {
        issue(['validTo'], 'validTo.beforeFrom');
      }

      const needed = MIN_CONDITIONS[values.conditionType];
      if (needed > 0) {
        if (values.conditions.length < needed) {
          issue(['conditions'], 'conditions.tooFew', { count: needed });
        }
        const seen = new Set<string>();
        values.conditions.forEach((entry, index) => {
          if (!entry.targetId)
            issue(['conditions', index, 'targetId'], 'conditions.targetRequired');
          else if (seen.has(entry.targetId)) {
            issue(['conditions', index, 'targetId'], 'conditions.repeated');
          }
          seen.add(entry.targetId);
          if (entry.minQuantity !== '' && !isPositive(entry.minQuantity)) {
            issue(['conditions', index, 'minQuantity'], 'conditions.minQuantityPositive');
          }
        });
      }

      if (tiered) {
        if (values.tiers.length === 0) issue(['tiers'], 'tiers.required');
        values.tiers.forEach((band, index) => {
          if (!isPositive(band.fromQuantity)) {
            issue(['tiers', index, 'fromQuantity'], 'tiers.fromRequired');
          } else if (
            band.toQuantity !== '' &&
            Number(band.toQuantity) < Number(band.fromQuantity)
          ) {
            issue(['tiers', index, 'toQuantity'], 'tiers.toBelowFrom');
          }
          if (!isPositive(band.value)) issue(['tiers', index, 'value'], 'tiers.valueRequired');
          else if (percentage && Number(band.value) > PERCENTAGE_MAX) {
            issue(['tiers', index, 'value'], 'actionValue.percentageMax', { max: PERCENTAGE_MAX });
          }
          const overlaps = values.tiers
            .slice(0, index)
            .some((earlier) => tiersOverlap(earlier, band));
          if (overlaps && isPositive(band.fromQuantity)) {
            issue(['tiers', index, 'fromQuantity'], 'tiers.overlap');
          }
        });
      }
    });
}

export type PromotionValues = z.infer<ReturnType<typeof promotionSchema>>;
export type PromotionConditionValues = PromotionValues['conditions'][number];
export type PromotionTierValues = PromotionValues['tiers'][number];

// Both bounds are inclusive and an empty upper bound is open, the way the API reads a band.
function tiersOverlap(a: PromotionTierValues, b: PromotionTierValues): boolean {
  const aBelowB = a.toQuantity !== '' && Number(a.toQuantity) < Number(b.fromQuantity);
  const bBelowA = b.toQuantity !== '' && Number(b.toQuantity) < Number(a.fromQuantity);
  return !aBelowB && !bBelowA;
}
