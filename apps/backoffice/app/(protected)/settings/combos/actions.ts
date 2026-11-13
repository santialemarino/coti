'use server';

import { revalidatePath } from 'next/cache';

import { comboSchema, type ComboValues } from '@/app/(protected)/settings/combos/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import type { Combo } from '@/lib/api/combos';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface ComboResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function createCombo(values: ComboValues): Promise<ComboResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = comboSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  return write({ path: '/v1/combos', method: 'POST', body: bodyOf(parsed.data) });
}

export async function updateCombo(comboId: string, values: ComboValues): Promise<ComboResult> {
  const parsed = comboSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  return write({
    path: `/v1/combos/${encodeURIComponent(comboId)}`,
    method: 'PUT',
    body: bodyOf(parsed.data),
  });
}

// Switching a combo on or off resends it as the list holds it: `PUT` replaces the whole record.
export async function setComboActive(combo: Combo, isActive: boolean): Promise<ComboResult> {
  return write({
    path: `/v1/combos/${encodeURIComponent(combo.id)}`,
    method: 'PUT',
    body: {
      name: combo.name,
      description: combo.description ?? undefined,
      is_active: isActive,
      items: combo.items.map((item) => ({ product_id: item.productId, quantity: item.quantity })),
      branch_ids: combo.branchIds,
    },
  });
}

function bodyOf(values: ComboValues) {
  return {
    name: values.name,
    // Omitted rather than empty: the API's optional fields are pointers, and only nil is skipped.
    description: values.description || undefined,
    is_active: values.isActive,
    items: values.items.map((item) => ({ product_id: item.productId, quantity: item.quantity })),
    branch_ids: values.branchIds,
  };
}

interface ComboWrite {
  path: string;
  method: 'POST' | 'PUT';
  body: Record<string, unknown>;
}

async function write(request: ComboWrite): Promise<ComboResult> {
  try {
    await apiRequest(request);
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.comboSettings, 'layout');
  return { ok: true };
}
