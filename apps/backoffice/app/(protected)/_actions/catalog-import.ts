'use server';

import { apiFetch, apiRequest, toApiError } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

interface CatalogImportRowRaw {
  row_number: number;
  code: string;
  name: string;
  description: string;
  unit: string;
  family: string;
  subgroup: string | null;
  price: string;
  min_price: string | null;
  errors: string[] | null;
}

interface CatalogImportPreviewRaw {
  rows: CatalogImportRowRaw[];
  valid_rows: number;
  invalid_rows: number;
  can_confirm: boolean;
  previewed_at: string;
}

interface ConfirmCatalogImportRaw {
  imported_rows: number;
  skipped_rows: number;
}

export interface CatalogImportRow {
  rowNumber: number;
  code: string;
  name: string;
  description: string;
  unit: string;
  family: string;
  subgroup: string | null;
  price: string;
  minPrice: string | null;
  errors: string[];
}

export interface CatalogImportPreview {
  branchId: string;
  rows: CatalogImportRow[];
  validRows: number;
  invalidRows: number;
  canConfirm: boolean;
  previewedAt: string;
}

export type CatalogPreviewResult =
  | { ok: true; preview: CatalogImportPreview }
  | { ok: false; error: ApiErrorCode };

export type CatalogConfirmResult =
  | { ok: true; importedRows: number; skippedRows: number }
  | { ok: false; error: ApiErrorCode };

export type CatalogTemplateResult =
  | { ok: true; filename: string; contentBase64: string }
  | { ok: false; error: ApiErrorCode };

function mapCatalogImportRow(raw: CatalogImportRowRaw): CatalogImportRow {
  return {
    rowNumber: raw.row_number,
    code: raw.code,
    name: raw.name,
    description: raw.description,
    unit: raw.unit,
    family: raw.family,
    subgroup: raw.subgroup,
    price: raw.price,
    minPrice: raw.min_price,
    errors: raw.errors ?? [],
  };
}

export async function previewCatalogImport(
  branchId: string,
  formData: FormData,
): Promise<CatalogPreviewResult> {
  const file = formData.get('file');
  if (!branchId || !(file instanceof File) || file.size === 0) {
    return { ok: false, error: 'INVALID_BODY' };
  }

  const payload = new FormData();
  payload.set('file', file);
  try {
    const raw = await apiRequest<CatalogImportPreviewRaw>({
      path: '/v1/products/import/preview',
      method: 'POST',
      formData: payload,
      branchId,
    });
    return {
      ok: true,
      preview: {
        branchId,
        rows: raw.rows.map(mapCatalogImportRow),
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

export async function downloadCatalogTemplate(branchId: string): Promise<CatalogTemplateResult> {
  if (!branchId) return { ok: false, error: 'INVALID_BODY' };
  const response = await apiFetch({ path: '/v1/products/export', branchId });
  if (!response.ok) return { ok: false, error: (await toApiError(response)).code };

  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? 'catalogo.xlsx';
  const content = Buffer.from(await response.arrayBuffer()).toString('base64');
  return { ok: true, filename, contentBase64: content };
}

export async function confirmCatalogImport(
  preview: CatalogImportPreview,
): Promise<CatalogConfirmResult> {
  try {
    const reviewedRows = preview.rows.filter((row) => row.errors.length === 0);
    const raw = await apiRequest<ConfirmCatalogImportRaw>({
      path: '/v1/products/import/confirm',
      method: 'POST',
      branchId: preview.branchId,
      body: {
        rows: reviewedRows.map((row) => ({
          code: row.code,
          name: row.name,
          description: row.description,
          unit: row.unit,
          family: row.family,
          subgroup: row.subgroup,
          price: row.price,
          min_price: row.minPrice,
        })),
      },
    });
    return {
      ok: true,
      importedRows: raw.imported_rows,
      skippedRows: preview.invalidRows + raw.skipped_rows,
    };
  } catch (error) {
    return { ok: false, error: errorCodeOf(error) };
  }
}
