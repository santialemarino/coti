import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { ApiError } from '@/lib/api/errors';

// --- Raw types (API JSON shape, snake_case) ---

interface ComboItemRaw {
  product_id: string;
  quantity: string;
  product_code: string | null;
  product_name: string;
  product_unit: string | null;
  product_is_active: boolean;
}

interface ComboRaw {
  id: string;
  name: string;
  description: string | null;
  is_active: boolean;
  items: ComboItemRaw[] | null;
  branch_ids: string[] | null;
}

// --- Frontend types (camelCase) ---

export interface ComboItem {
  productId: string;
  quantity: string;
  productCode: string | null;
  productName: string;
  productUnit: string | null;
  productIsActive: boolean;
}

export interface Combo {
  id: string;
  name: string;
  description: string | null;
  isActive: boolean;
  items: ComboItem[];
  branchIds: string[];
}

// --- Mappers ---

function mapCombo(raw: ComboRaw): Combo {
  return {
    id: raw.id,
    name: raw.name,
    description: raw.description,
    isActive: raw.is_active,
    items: (raw.items ?? []).map((item) => ({
      productId: item.product_id,
      quantity: item.quantity,
      productCode: item.product_code,
      productName: item.product_name,
      productUnit: item.product_unit,
      productIsActive: item.product_is_active,
    })),
    branchIds: raw.branch_ids ?? [],
  };
}

// --- API functions ---

// Every combo of the account, inactive ones included, by name. Admin only.
export async function getCombos(): Promise<Combo[]> {
  const { items } = await apiRequest<{ items: ComboRaw[] }>({
    path: '/v1/combos',
    branchScoped: false,
  });
  return items.map(mapCombo);
}

// One combo, or null when the account has none with that id (a malformed id included).
export async function getCombo(id: string): Promise<Combo | null> {
  try {
    return mapCombo(
      await apiRequest<ComboRaw>({
        path: `/v1/combos/${encodeURIComponent(id)}`,
        branchScoped: false,
      }),
    );
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.status === 400)) return null;
    throw error;
  }
}
