'use server';

import { revalidatePath } from 'next/cache';

import { clientTagRuleSchema } from '@/app/(protected)/settings/client-tags/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import type { ClientTagRule } from '@/lib/api/client-tag-rules';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

export async function updateClientTagRule(input: ClientTagRule): Promise<ApiErrorCode | null> {
  await requireAdmin();
  const parsed = clientTagRuleSchema().safeParse(input);
  if (!parsed.success) return 'INVALID_INPUT';
  const { kind, enabled, minimum, windowDays } = parsed.data;

  try {
    await apiRequest<void>({
      path: `/v1/tags/rules/${kind}`,
      method: 'PUT',
      branchScoped: false,
      body: { enabled, minimum, window_days: windowDays },
    });
  } catch (error) {
    return errorCodeOf(error);
  }
  revalidatePath(ROUTES.clients);
  revalidatePath(ROUTES.clientDetail('[id]'), 'page');
  return null;
}
