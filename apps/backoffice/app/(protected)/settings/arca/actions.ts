'use server';

import { revalidatePath } from 'next/cache';

import {
  arcaIdentitySchema,
  type ARCAIdentityValues,
} from '@/app/(protected)/settings/arca/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

interface SetupResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function createARCASetup(values: ARCAIdentityValues): Promise<SetupResult> {
  await requireAdmin();
  const parsed = arcaIdentitySchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  try {
    await apiRequest({
      path: '/v1/arca/setup',
      method: 'POST',
      body: { tax_id: parsed.data.taxId },
      branchScoped: false,
    });
    revalidatePath(ROUTES.arcaSettings);
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function uploadARCACertificate(certificate: string): Promise<SetupResult> {
  await requireAdmin();
  if (!certificate || certificate.length > 16384) return { error: 'INVALID_BODY' };
  try {
    await apiRequest({
      path: '/v1/arca/setup/certificate',
      method: 'PUT',
      body: { certificate },
      branchScoped: false,
    });
    revalidatePath(ROUTES.arcaSettings);
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function disconnectARCA(): Promise<SetupResult> {
  await requireAdmin();
  try {
    await apiRequest({ path: '/v1/arca/setup', method: 'DELETE', branchScoped: false });
    revalidatePath(ROUTES.arcaSettings);
    return { ok: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}
