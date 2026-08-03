'use server';

import {
  forgotPasswordSchema,
  type ForgotPasswordValues,
} from '@/app/(auth)/forgot-password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export interface ForgotPasswordResult {
  sent?: boolean;
  error?: 'unexpected';
  fieldError?: { field: 'email'; key: 'invalid' };
}

/*
 * The screen says the same thing whether or not the address is registered, which is
 * the API's contract and the point of the flow: any difference here would hand back
 * the enumeration the 202 was designed to withhold.
 */
export async function requestPasswordRecovery(
  values: ForgotPasswordValues,
): Promise<ForgotPasswordResult> {
  const parsed = forgotPasswordSchema().safeParse(values);
  if (!parsed.success) return { fieldError: { field: 'email', key: 'invalid' } };

  try {
    await apiRequest({
      path: '/v1/public/auth/forgot-password',
      method: 'POST',
      authenticated: false,
      body: { email: parsed.data.email },
    });
    return { sent: true };
  } catch (error) {
    if (errorCodeOf(error) === 'badRequest') {
      return { fieldError: { field: 'email', key: 'invalid' } };
    }
    return { error: 'unexpected' };
  }
}
