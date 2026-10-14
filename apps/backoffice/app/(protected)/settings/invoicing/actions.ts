'use server';

import { revalidatePath } from 'next/cache';

import {
  grossIncomeRegistrationOf,
  invoicingSettingsSchema,
  type InvoicingSettingsOutput,
  type InvoicingSettingsValues,
} from '@/app/(protected)/settings/invoicing/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import {
  mapInvoicingSettings,
  type InvoicingSettings,
  type InvoicingSettingsRaw,
} from '@/lib/api/invoicing';
import { requireAdmin } from '@/lib/auth/session';
import {
  pointsOfSaleBody,
  pointsOfSaleSchema,
  type PointOfSaleValue,
} from '@/lib/forms/points-of-sale';
import { rawText } from '@/lib/forms/validators';

export interface InvoicingSettingsResult {
  settings?: InvoicingSettings;
  error?: ApiErrorCode;
}

export async function updateInvoicingSettings(
  values: InvoicingSettingsValues,
): Promise<InvoicingSettingsResult> {
  await requireAdmin();
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
    revalidatePath(ROUTES.invoicingSettings, 'layout');
    return { settings: mapInvoicingSettings(raw) };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

// The profile, the IVA condition and every branch's point of sale are saved as one write.
function settingsBody(values: InvoicingSettingsOutput) {
  return {
    profile: {
      address: values.address,
      gross_income_registration: grossIncomeRegistrationOf(values),
      activity_started_on: values.activityStartedOn,
    },
    iva_condition: values.ivaCondition,
    prices_include_vat: values.pricesIncludeVat,
    branches: pointsOfSaleBody(values.branches),
  };
}

// The points of sale alone, saved from the ARCA guide without touching the rest of the settings.
export async function updatePointsOfSale(
  values: PointOfSaleValue[],
): Promise<InvoicingSettingsResult> {
  await requireAdmin();
  const parsed = pointsOfSaleSchema(rawText).safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    const raw = await apiRequest<InvoicingSettingsRaw>({
      path: '/v1/invoicing/points-of-sale',
      method: 'PUT',
      branchScoped: false,
      body: { branches: pointsOfSaleBody(parsed.data) },
    });
    revalidatePath(ROUTES.invoicingSettings, 'layout');
    return { settings: mapInvoicingSettings(raw) };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}
