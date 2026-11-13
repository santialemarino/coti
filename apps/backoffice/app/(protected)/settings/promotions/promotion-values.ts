import type { PromotionValues } from '@/app/(protected)/settings/promotions/form-schema';
import { argentinaDay } from '@/app/(protected)/settings/promotions/promotion-status';
import type { Promotion } from '@/lib/api/promotions';

// A new rule starts as the most common one: a percentage off chosen products, on from today on.
export const EMPTY_PROMOTION: PromotionValues = {
  name: '',
  description: '',
  branchId: '',
  conditionType: 'PER_ITEM',
  actionType: 'PERCENTAGE',
  actionValue: '',
  minTotal: '',
  validFrom: '',
  validTo: '',
  isActive: true,
  isExclusive: false,
  priority: '0',
  conditions: [{ kind: 'product', targetId: '', label: '', minQuantity: '' }],
  tiers: [],
};

// The form's values for a stored rule. Product names come from the catalog; the API sends ids.
export function promotionValues(
  promotion: Promotion,
  productNames: Map<string, string>,
): PromotionValues {
  return {
    name: promotion.name,
    description: promotion.description ?? '',
    branchId: promotion.branchId ?? '',
    conditionType: promotion.conditionType,
    actionType: promotion.actionType,
    actionValue: promotion.conditionType === 'QUANTITY_TIERED' ? '' : promotion.actionValue,
    minTotal: promotion.minTotal ?? '',
    validFrom: promotion.validFrom ? argentinaDay(promotion.validFrom) : '',
    validTo: promotion.validTo ? argentinaDay(promotion.validTo) : '',
    isActive: promotion.isActive,
    isExclusive: promotion.isExclusive,
    priority: String(promotion.priority),
    conditions: promotion.conditions.map((condition) => {
      if (condition.productId) {
        return {
          kind: 'product' as const,
          targetId: condition.productId,
          label: productNames.get(condition.productId) ?? '',
          minQuantity: condition.minQuantity ?? '',
        };
      }
      return {
        kind: condition.familyId ? ('family' as const) : ('subgroup' as const),
        targetId: condition.familyId ?? condition.subgroupId ?? '',
        label: '',
        minQuantity: condition.minQuantity ?? '',
      };
    }),
    tiers: promotion.tiers.map((tier) => ({
      fromQuantity: tier.fromQuantity,
      toQuantity: tier.toQuantity ?? '',
      value: tier.value,
    })),
  };
}
