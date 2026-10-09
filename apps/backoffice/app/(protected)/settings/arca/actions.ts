'use server';

import { revalidatePath } from 'next/cache';

import {
  arcaIdentitySchema,
  arcaPointOfSaleSchema,
  type ARCAIdentityValues,
  type ARCAPointOfSaleValues,
} from '@/app/(protected)/settings/arca/form-schema';
import { ROUTES } from '@/config/routes';
import { checkARCAConnection, type ARCAConnection } from '@/lib/api/arca-setup';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

interface SetupResult {
  ok?: true;
  error?: ApiErrorCode;
  connection?: ARCAConnection;
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

export async function verifyARCA(
  values: ARCAPointOfSaleValues,
  branchId: string,
): Promise<SetupResult> {
  await requireAdmin();
  const parsed = arcaPointOfSaleSchema().safeParse(values);
  if (!parsed.success || !zBranchId(branchId)) return { error: 'INVALID_BODY' };
  try {
    const connection = await checkARCAConnection(Number(parsed.data.pointOfSale), branchId);
    revalidatePath(ROUTES.arcaSettings);
    return { ok: true, connection };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

function zBranchId(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}
