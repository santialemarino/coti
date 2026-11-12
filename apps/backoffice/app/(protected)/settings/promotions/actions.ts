'use server';

import { revalidatePath } from 'next/cache';

import {
  promotionSchema,
  type PromotionValues,
} from '@/app/(protected)/settings/promotions/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import type { Promotion } from '@/lib/api/promotions';

// Argentina keeps one offset all year, so a calendar day maps to a fixed instant range.
const ARGENTINA_OFFSET = '-03:00';

export interface PromotionResult {
  ok?: true;
  id?: string;
  error?: ApiErrorCode;
}

export async function createPromotion(values: PromotionValues): Promise<PromotionResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = promotionSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({ path: '/v1/promotions', method: 'POST', body: bodyOf(parsed.data) });
}

export async function updatePromotion(
  promotionId: string,
  values: PromotionValues,
): Promise<PromotionResult> {
  const parsed = promotionSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({
    path: `/v1/promotions/${encodeURIComponent(promotionId)}`,
    method: 'PUT',
    body: bodyOf(parsed.data),
  });
}

/*
 * Turning a rule on or off is the same replace as an edit with the flag flipped, so it resends the
 * rule as the list holds it: `PUT` replaces the whole record, and a rule switched off keeps its
 * definition for the day it is switched back on.
 */
export async function setPromotionActive(
  promotion: Promotion,
  isActive: boolean,
): Promise<PromotionResult> {
  const tiered = promotion.conditionType === 'QUANTITY_TIERED';
  return write({
    path: `/v1/promotions/${encodeURIComponent(promotion.id)}`,
    method: 'PUT',
    body: {
      name: promotion.name,
      description: promotion.description ?? undefined,
      branch_id: promotion.branchId ?? undefined,
      condition_type: promotion.conditionType,
      action_type: promotion.actionType,
      action_value: tiered ? undefined : promotion.actionValue,
      min_total: promotion.minTotal ?? undefined,
      valid_from: promotion.validFrom ?? undefined,
      valid_to: promotion.validTo ?? undefined,
      is_active: isActive,
      is_exclusive: promotion.isExclusive,
      priority: promotion.priority,
      conditions: promotion.conditions.map((condition) => ({
        product_id: condition.productId ?? undefined,
        family_id: condition.familyId ?? undefined,
        subgroup_id: condition.subgroupId ?? undefined,
        min_quantity: condition.minQuantity ?? undefined,
      })),
      tiers: promotion.tiers.map((tier) => ({
        from_quantity: tier.fromQuantity,
        to_quantity: tier.toQuantity ?? undefined,
        value: tier.value,
      })),
    },
  });
}

function bodyOf(values: PromotionValues) {
  const tiered = values.conditionType === 'QUANTITY_TIERED';
  const onTotal = values.conditionType === 'ON_TOTAL';
  return {
    name: values.name,
    // Omitted rather than empty: the API's optional fields are pointers, and only nil is skipped.
    description: values.description || undefined,
    branch_id: values.branchId || undefined,
    condition_type: values.conditionType,
    action_type: values.actionType,
    action_value: tiered ? undefined : values.actionValue,
    min_total: onTotal && values.minTotal ? values.minTotal : undefined,
    // A day runs from its first second to its last, so a one-day promotion is valid all day.
    valid_from: values.validFrom ? `${values.validFrom}T00:00:00${ARGENTINA_OFFSET}` : undefined,
    valid_to: values.validTo ? `${values.validTo}T23:59:59${ARGENTINA_OFFSET}` : undefined,
    is_active: values.isActive,
    is_exclusive: values.isExclusive,
    priority: Number(values.priority),
    conditions: onTotal
      ? []
      : values.conditions.map((condition) => ({
          product_id: condition.kind === 'product' ? condition.targetId : undefined,
          family_id: condition.kind === 'family' ? condition.targetId : undefined,
          subgroup_id: condition.kind === 'subgroup' ? condition.targetId : undefined,
          min_quantity: condition.minQuantity || undefined,
        })),
    tiers: tiered
      ? values.tiers.map((tier) => ({
          from_quantity: tier.fromQuantity,
          to_quantity: tier.toQuantity || undefined,
          value: tier.value,
        }))
      : [],
  };
}

interface PromotionWrite {
  path: string;
  method: 'POST' | 'PUT';
  body: Record<string, unknown>;
}

async function write(request: PromotionWrite): Promise<PromotionResult> {
  let id: string | undefined;
  try {
    ({ id } = await apiRequest<{ id: string }>(request));
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.promotionSettings, 'layout');
  return { ok: true, id };
}
