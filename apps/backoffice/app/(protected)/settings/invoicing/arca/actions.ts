'use server';

import { revalidatePath } from 'next/cache';

import { arcaCertificateSchema } from '@/app/(protected)/settings/invoicing/arca/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

interface SetupResult {
  ok?: true;
  error?: ApiErrorCode;
}

// The request is generated for the account's own CUIT, so nothing about the issuer is sent.
export async function createARCASetup(): Promise<SetupResult> {
  await requireAdmin();
  return run({ path: '/v1/arca/setup', method: 'POST' });
}

export async function uploadARCACertificate(certificate: string): Promise<SetupResult> {
  await requireAdmin();
  const parsed = arcaCertificateSchema().safeParse({ certificate });
  if (!parsed.success) return { error: 'INVALID_BODY' };
  return run({
    path: '/v1/arca/setup/certificate',
    method: 'PUT',
    body: { certificate: parsed.data.certificate },
  });
}

export async function disconnectARCA(): Promise<SetupResult> {
  await requireAdmin();
  return run({ path: '/v1/arca/setup', method: 'DELETE' });
}

async function run(request: {
  path: string;
  method: 'POST' | 'PUT' | 'DELETE';
  body?: unknown;
}): Promise<SetupResult> {
  try {
    await apiRequest({ ...request, branchScoped: false });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  // The invoicing page shows the connection's state too.
  revalidatePath(ROUTES.invoicingSettings, 'layout');
  return { ok: true };
}
