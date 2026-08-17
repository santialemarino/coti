'use server';

import {
  forgotPasswordSchema,
  type ForgotPasswordValues,
} from '@/app/(auth)/forgot-password/form-schema';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface ForgotPasswordResult {
  sent?: boolean;
  error?: ApiErrorCode;
  /* The address is the only thing this screen can be wrong about. */
  field?: 'email';
}

/* Which field a refusal belongs on. A code absent from the map belongs to the form. */
const FIELD_FOR: Partial<Record<ApiErrorCode, 'email'>> = { INVALID_BODY: 'email' };

/*
 * The screen says the same thing whether or not the address is registered, which is
 * the API's contract and the point of the flow: any difference here would hand back
 * the enumeration the 202 was designed to withhold. The per-address cap answers 202 for
 * the same reason; the 429 that does arrive is the caller's own allowance.
 */
export async function requestPasswordRecovery(
  values: ForgotPasswordValues,
): Promise<ForgotPasswordResult> {
  const parsed = forgotPasswordSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY', field: 'email' };

  try {
    await apiRequest({
      path: '/v1/public/auth/forgot-password',
      method: 'POST',
      authenticated: false,
      body: { email: parsed.data.email },
    });
    return { sent: true };
  } catch (error) {
    const code = errorCodeOf(error);
    return { error: code, field: FIELD_FOR[code] };
  }
}
