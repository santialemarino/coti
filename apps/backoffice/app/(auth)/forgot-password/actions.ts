'use server';

import { forgotPasswordSchema } from '@/app/(auth)/forgot-password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export interface ForgotPasswordState {
  sent?: boolean;
  error?: 'invalidEmail' | 'unexpected';
  // React resets a form once its action resolves; the field re-seeds itself from here.
  email?: string;
}

/*
 * The screen says the same thing whether or not the address is registered, which is
 * the API's contract and the point of the flow: any difference here would hand back
 * the enumeration the 202 was designed to withhold.
 */
export async function requestPasswordRecovery(
  _previous: ForgotPasswordState,
  formData: FormData,
): Promise<ForgotPasswordState> {
  const email = String(formData.get('email') ?? '');
  const parsed = forgotPasswordSchema.safeParse({ email });
  if (!parsed.success) return { error: 'invalidEmail', email };

  try {
    await apiRequest({
      path: '/v1/public/auth/forgot-password',
      method: 'POST',
      authenticated: false,
      body: { email: parsed.data.email },
    });
    return { sent: true };
  } catch (error) {
    return { error: errorCodeOf(error) === 'badRequest' ? 'invalidEmail' : 'unexpected', email };
  }
}
