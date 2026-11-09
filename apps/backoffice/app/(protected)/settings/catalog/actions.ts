'use server';

import { revalidatePath } from 'next/cache';

import { productSchema, type ProductValues } from '@/app/(protected)/settings/catalog/form-schema';
import {
  productPriceSchema,
  type ProductPriceValues,
} from '@/app/(protected)/settings/catalog/price-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

interface ProductResponseRaw {
  id: string;
}

interface ProductAlternativeRaw {
  id: string;
  alternative_product_id: string;
  type: 'EQUIVALENT' | 'ECONOMY' | 'PREMIUM';
  product: {
    code: string | null;
    canonical_name: string;
    unit: string | null;
  } | null;
}

interface ProductAlternativeListRaw {
  items: ProductAlternativeRaw[];
}

export interface ProductWriteResult {
  ok?: true;
  productId?: string;
  recommendationId?: string;
  error?: ApiErrorCode;
}

export interface ProductRecommendation {
  id: string;
  productId: string;
  type: 'ECONOMY' | 'PREMIUM';
  name: string;
  code: string | null;
  unit: string | null;
}

export type ProductRecommendationResult =
  | { ok: true; items: ProductRecommendation[] }
  | { ok?: false; error: ApiErrorCode };

interface ProductPriceRaw {
  price: string;
  min_price: string | null;
  currency: string;
  valid_from: string;
  valid_to: string | null;
}

export type ProductPriceResult =
  | { ok: true; current: { price: string; minPrice: string | null; currency: string } | null }
  | { ok?: false; error: ApiErrorCode };

export async function getProductPrice(
  productId: string,
  branchId: string,
): Promise<ProductPriceResult> {
  if (!branchId) return { error: 'INVALID_BODY' };
  try {
    const response = await apiRequest<{ items: ProductPriceRaw[] }>({
      path: `/v1/products/${productId}/prices`,
      branchId,
    });
    const now = Date.now();
    const current = response.items.find(
      (price) =>
        Date.parse(price.valid_from) <= now &&
        (!price.valid_to || Date.parse(price.valid_to) > now),
    );
    return {
      ok: true,
      current: current
        ? {
            price: current.price,
            minPrice: current.min_price,
            currency: current.currency,
          }
        : null,
    };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function setProductPrice(
  productId: string,
  values: ProductPriceValues,
): Promise<ProductWriteResult> {
  const parsed = productPriceSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  try {
    await apiRequest({
      path: `/v1/products/${productId}/prices`,
      method: 'POST',
      branchId: parsed.data.branchId,
      body: {
        price: parsed.data.price,
        min_price: parsed.data.minPrice || null,
        currency: parsed.data.currency,
      },
    });
    revalidatePath('/settings/catalog');
    revalidatePath(ROUTES.rfqs, 'layout');
    return { ok: true, productId };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function listProductRecommendations(
  productId: string,
): Promise<ProductRecommendationResult> {
  try {
    const response = await apiRequest<ProductAlternativeListRaw>({
      path: `/v1/products/${productId}/alternatives`,
      branchScoped: false,
    });
    return {
      ok: true,
      items: response.items.flatMap((item) => {
        if ((item.type !== 'ECONOMY' && item.type !== 'PREMIUM') || !item.product) return [];
        return [
          {
            id: item.id,
            productId: item.alternative_product_id,
            type: item.type,
            name: item.product.canonical_name,
            code: item.product.code,
            unit: item.product.unit,
          },
        ];
      }),
    };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function setProductRecommendation(
  productId: string,
  recommendationType: 'ECONOMY' | 'PREMIUM',
  alternativeProductId: string,
): Promise<ProductWriteResult> {
  try {
    const recommendation = await apiRequest<ProductAlternativeRaw>({
      path: `/v1/products/${productId}/recommendations/${recommendationType}`,
      method: 'PUT',
      branchScoped: false,
      body: { alternative_product_id: alternativeProductId },
    });
    revalidatePath('/settings/catalog');
    return { ok: true, productId, recommendationId: recommendation.id };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function removeProductRecommendation(
  productId: string,
  alternativeId: string,
): Promise<ProductWriteResult> {
  try {
    await apiRequest({
      path: `/v1/products/${productId}/alternatives/${alternativeId}`,
      method: 'DELETE',
      branchScoped: false,
    });
    revalidatePath('/settings/catalog');
    return { ok: true, productId };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function createProduct(values: ProductValues): Promise<ProductWriteResult> {
  const parsed = productSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    const product = await apiRequest<ProductResponseRaw>({
      path: '/v1/products',
      method: 'POST',
      branchScoped: false,
      body: {
        ...productBody(parsed.data),
        price: parsed.data.price || null,
        min_price: parsed.data.minPrice || null,
      },
    });
    revalidatePath('/settings/catalog');
    return { ok: true, productId: product.id };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function updateProduct(
  productId: string,
  values: ProductValues,
): Promise<ProductWriteResult> {
  const parsed = productSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return writeProduct(productId, { ...productBody(parsed.data), is_active: parsed.data.isActive });
}

export async function deactivateProduct(productId: string): Promise<ProductWriteResult> {
  try {
    await apiRequest({
      path: `/v1/products/${productId}`,
      method: 'DELETE',
      branchScoped: false,
    });
    revalidatePath('/settings/catalog');
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function reactivateProduct(
  productId: string,
  values: ProductValues,
): Promise<ProductWriteResult> {
  const parsed = productSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return writeProduct(productId, { ...productBody(parsed.data), is_active: true });
}

export async function uploadProductImage(
  productId: string,
  formData: FormData,
): Promise<ProductWriteResult> {
  const file = formData.get('file');
  if (!(file instanceof File) || file.size === 0) return { error: 'INVALID_BODY' };

  const payload = new FormData();
  payload.set('file', file);
  try {
    await apiRequest({
      path: `/v1/products/${productId}/image`,
      method: 'POST',
      branchScoped: false,
      formData: payload,
    });
    revalidatePath('/settings/catalog');
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

function productBody(values: ProductValues) {
  return {
    code: values.code || null,
    canonical_name: values.name,
    description: values.description || null,
    unit: values.unit,
    family_id: values.familyId,
    subgroup_id: values.subgroupId || null,
    vat_rate: values.vatRate,
  };
}

async function writeProduct(productId: string, body: unknown): Promise<ProductWriteResult> {
  try {
    await apiRequest({
      path: `/v1/products/${productId}`,
      method: 'PUT',
      branchScoped: false,
      body,
    });
    revalidatePath('/settings/catalog');
    return { ok: true, productId };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}
