'use server';

import { resetPasswordSchema } from '@/app/(auth)/reset-password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export type ResetPasswordErrorKey = 'mismatch' | 'tooShort' | 'invalidLink' | 'unexpected';

export interface ResetPasswordState {
  done?: boolean;
  error?: ResetPasswordErrorKey;
}

export async function resetPassword(
  _previous: ResetPasswordState,
  formData: FormData,
): Promise<ResetPasswordState> {
  const values = {
    token: String(formData.get('token') ?? ''),
    newPassword: String(formData.get('newPassword') ?? ''),
    confirmPassword: String(formData.get('confirmPassword') ?? ''),
  };
  const parsed = resetPasswordSchema.safeParse(values);
  if (!parsed.success) {
    return { error: values.newPassword === values.confirmPassword ? 'tooShort' : 'mismatch' };
  }

  try {
    await apiRequest({
      path: '/v1/public/auth/reset-password',
      method: 'POST',
      authenticated: false,
      body: { token: parsed.data.token, new_password: parsed.data.newPassword },
    });
    return { done: true };
  } catch (error) {
    const code = errorCodeOf(error);
    // The API answers 401 for a link that is unknown, expired or already used, and
    // the screen keeps them together for the same reason it does.
    if (code === 'unauthenticated') return { error: 'invalidLink' };
    if (code === 'unprocessable') return { error: 'tooShort' };
    return { error: 'unexpected' };
  }
}
