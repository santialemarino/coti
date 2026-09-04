'use server';

import { apiFetch, apiRequest, toApiError } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

interface ProductPriceImportRowRaw {
  row_number: number;
  code: string;
  product_name: string;
  current_price: string | null;
  current_min_price: string | null;
  price: string;
  min_price: string | null;
  currency: string;
  errors: string[] | null;
}

interface ProductPriceImportPreviewRaw {
  rows: ProductPriceImportRowRaw[];
  valid_rows: number;
  invalid_rows: number;
  can_confirm: boolean;
  previewed_at: string;
}

interface ConfirmProductPriceImportRaw {
  imported_rows: number;
}

export interface ProductPriceImportRow {
  rowNumber: number;
  code: string;
  productName: string;
  currentPrice: string | null;
  currentMinPrice: string | null;
  price: string;
  minPrice: string | null;
  currency: string;
  errors: string[];
}

export interface ProductPriceImportPreview {
  branchId: string;
  rows: ProductPriceImportRow[];
  validRows: number;
  invalidRows: number;
  canConfirm: boolean;
  previewedAt: string;
}

export type PriceImportActionResult =
  | { ok: true; preview: ProductPriceImportPreview }
  | { ok: false; error: ApiErrorCode };

export type ConfirmPriceImportResult =
  | { ok: true; importedRows: number }
  | { ok: false; error: ApiErrorCode };

export type ExportPricesResult =
  | { ok: true; filename: string; contentBase64: string }
  | { ok: false; error: ApiErrorCode };

function mapProductPriceImportRow(raw: ProductPriceImportRowRaw): ProductPriceImportRow {
  return {
    rowNumber: raw.row_number,
    code: raw.code,
    productName: raw.product_name,
    currentPrice: raw.current_price,
    currentMinPrice: raw.current_min_price,
    price: raw.price,
    minPrice: raw.min_price,
    currency: raw.currency,
    errors: raw.errors ?? [],
  };
}

/*
 * The branch travels with every call rather than being read from the active one, so a preview
 * confirmed after a branch switch still writes where it was prepared. The API refuses one the
 * caller does not reach.
 */
export async function previewPriceImport(
  branchId: string,
  formData: FormData,
): Promise<PriceImportActionResult> {
  const file = formData.get('file');
  if (!branchId || !(file instanceof File) || file.size === 0) {
    return { ok: false, error: 'INVALID_BODY' };
  }

  const payload = new FormData();
  payload.set('file', file);
  try {
    const raw = await apiRequest<ProductPriceImportPreviewRaw>({
      path: '/v1/product-prices/import/preview',
      method: 'POST',
      formData: payload,
      branchId,
    });
    return {
      ok: true,
      preview: {
        branchId,
        rows: raw.rows.map(mapProductPriceImportRow),
        validRows: raw.valid_rows,
        invalidRows: raw.invalid_rows,
        canConfirm: raw.can_confirm,
        previewedAt: raw.previewed_at,
      },
    };
  } catch (error) {
    return { ok: false, error: errorCodeOf(error) };
  }
}

export async function exportPrices(branchId: string): Promise<ExportPricesResult> {
  if (!branchId) return { ok: false, error: 'INTERNAL' };
  // The raw response, because the filename travels in a header and the body is a
  // spreadsheet rather than JSON.
  const response = await apiFetch({ path: '/v1/product-prices/export', branchId });
  // A refusal is still the JSON envelope, so it carries its code like every other call.
  if (!response.ok) return { ok: false, error: (await toApiError(response)).code };

  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? 'precios.xlsx';
  const content = Buffer.from(await response.arrayBuffer()).toString('base64');
  return { ok: true, filename, contentBase64: content };
}

export async function confirmPriceImport(
  preview: ProductPriceImportPreview,
): Promise<ConfirmPriceImportResult> {
  try {
    const raw = await apiRequest<ConfirmProductPriceImportRaw>({
      path: '/v1/product-prices/import/confirm',
      method: 'POST',
      branchId: preview.branchId,
      body: {
        rows: preview.rows.map((row) => ({
          code: row.code,
          price: row.price,
          min_price: row.minPrice,
        })),
      },
    });
    return { ok: true, importedRows: raw.imported_rows };
  } catch (error) {
    return { ok: false, error: errorCodeOf(error) };
  }
}
