import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { ApiError } from '@/lib/api/errors';
import type { PromotionActionType, PromotionConditionType } from '@/lib/constants/promotions';

// --- Raw types (API JSON shape, snake_case) ---

interface PromotionConditionRaw {
  product_id: string | null;
  family_id: string | null;
  subgroup_id: string | null;
  min_quantity: string | null;
}

interface PromotionTierRaw {
  from_quantity: string;
  to_quantity: string | null;
  value: string;
}

interface PromotionRaw {
  id: string;
  branch_id: string | null;
  name: string;
  description: string | null;
  condition_type: PromotionConditionType;
  action_type: PromotionActionType;
  action_value: string;
  min_total: string | null;
  valid_from: string | null;
  valid_to: string | null;
  is_active: boolean;
  is_exclusive: boolean;
  priority: number;
  conditions: PromotionConditionRaw[] | null;
  tiers: PromotionTierRaw[] | null;
}

interface PromotionListRaw {
  items: PromotionRaw[];
}

// --- Frontend types (camelCase) ---

export interface PromotionCondition {
  productId: string | null;
  familyId: string | null;
  subgroupId: string | null;
  minQuantity: string | null;
}

export interface PromotionTier {
  fromQuantity: string;
  toQuantity: string | null;
  value: string;
}

export interface Promotion {
  id: string;
  branchId: string | null;
  name: string;
  description: string | null;
  conditionType: PromotionConditionType;
  actionType: PromotionActionType;
  actionValue: string;
  minTotal: string | null;
  validFrom: string | null;
  validTo: string | null;
  isActive: boolean;
  isExclusive: boolean;
  priority: number;
  conditions: PromotionCondition[];
  tiers: PromotionTier[];
}

// --- Mappers ---

function mapPromotion(raw: PromotionRaw): Promotion {
  return {
    id: raw.id,
    branchId: raw.branch_id,
    name: raw.name,
    description: raw.description,
    conditionType: raw.condition_type,
    actionType: raw.action_type,
    actionValue: raw.action_value,
    minTotal: raw.min_total,
    validFrom: raw.valid_from,
    validTo: raw.valid_to,
    isActive: raw.is_active,
    isExclusive: raw.is_exclusive,
    priority: raw.priority,
    conditions: (raw.conditions ?? []).map((condition) => ({
      productId: condition.product_id,
      familyId: condition.family_id,
      subgroupId: condition.subgroup_id,
      minQuantity: condition.min_quantity,
    })),
    tiers: (raw.tiers ?? []).map((tier) => ({
      fromQuantity: tier.from_quantity,
      toQuantity: tier.to_quantity,
      value: tier.value,
    })),
  };
}

// --- API functions ---

// Every rule of the account, inactive ones included, highest priority first. Admin only.
export async function getPromotions(): Promise<Promotion[]> {
  const { items } = await apiRequest<PromotionListRaw>({
    path: '/v1/promotions',
    branchScoped: false,
  });
  return items.map(mapPromotion);
}

// One rule, or null when the account has none with that id (a malformed id included).
export async function getPromotion(id: string): Promise<Promotion | null> {
  try {
    const raw = await apiRequest<PromotionRaw>({
      path: `/v1/promotions/${encodeURIComponent(id)}`,
      branchScoped: false,
    });
    return mapPromotion(raw);
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.status === 400)) return null;
    throw error;
  }
}
