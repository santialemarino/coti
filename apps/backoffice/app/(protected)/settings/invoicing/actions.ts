'use server';

import { revalidatePath } from 'next/cache';

import {
  invoicingSettingsSchema,
  type InvoicingSettingsValues,
} from '@/app/(protected)/settings/invoicing/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import {
  mapArcaCredential,
  mapInvoicingSettings,
  type ArcaCredential,
  type ArcaCredentialResponseRaw,
  type InvoicingSettings,
  type InvoicingSettingsRaw,
} from '@/lib/api/invoicing';

export interface InvoicingSettingsResult {
  settings?: InvoicingSettings;
  error?: ApiErrorCode;
}

export interface ArcaCredentialResult {
  ok?: true;
  credential?: ArcaCredential;
  error?: ApiErrorCode;
}

export async function updateInvoicingSettings(
  values: InvoicingSettingsValues,
): Promise<InvoicingSettingsResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = invoicingSettingsSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    const raw = await apiRequest<InvoicingSettingsRaw>({
      path: '/v1/invoicing/settings',
      method: 'PUT',
      branchScoped: false,
      body: settingsBody(parsed.data),
    });
    revalidatePath(ROUTES.invoicingSettings);
    return { settings: mapInvoicingSettings(raw) };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function uploadArcaCredentials(formData: FormData): Promise<ArcaCredentialResult> {
  const certificate = formData.get('certificate');
  const privateKey = formData.get('private_key');
  if (!isFile(certificate) || !isFile(privateKey)) return { error: 'INVALID_BODY' };

  const payload = new FormData();
  payload.set('certificate', certificate);
  payload.set('private_key', privateKey);
  try {
    const raw = await apiRequest<ArcaCredentialResponseRaw>({
      path: '/v1/invoicing/credentials',
      method: 'PUT',
      branchScoped: false,
      formData: payload,
    });
    revalidatePath(ROUTES.invoicingSettings);
    return { ok: true, credential: mapArcaCredential(raw) };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function deleteArcaCredentials(): Promise<ArcaCredentialResult> {
  try {
    await apiRequest({ path: '/v1/invoicing/credentials', method: 'DELETE', branchScoped: false });
    revalidatePath(ROUTES.invoicingSettings);
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

function settingsBody(values: InvoicingSettingsValues) {
  return {
    iva_condition: values.ivaCondition || null,
    prices_include_vat: values.pricesIncludeVat,
    branches: values.branches.map((branch) => ({
      branch_id: branch.branchId,
      point_of_sale: branch.pointOfSale === '' ? null : Number(branch.pointOfSale),
    })),
  };
}

function isFile(value: FormDataEntryValue | null): value is File {
  return value instanceof File && value.size > 0;
}
