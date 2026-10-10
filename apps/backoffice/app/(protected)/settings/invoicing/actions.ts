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
  mapInvoicingSettings,
  type ArcaCredential,
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

function settingsBody(values: InvoicingSettingsValues) {
  return {
    profile: {
      address: values.address ?? '',
      gross_income_registration: values.grossIncomeRegistration ?? '',
      activity_started_on: values.activityStartedOn ?? '',
    },
    iva_condition: values.ivaCondition || null,
    prices_include_vat: values.pricesIncludeVat,
    branches: [],
  };
}
