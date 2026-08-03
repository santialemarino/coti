'use server';

import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export interface ResetPasswordResult {
  done?: boolean;
  error?: 'invalidLink' | 'unexpected';
  fieldError?: { field: 'newPassword'; key: 'tooShort' };
}

export async function resetPassword(values: ResetPasswordValues): Promise<ResetPasswordResult> {
  const parsed = resetPasswordSchema().safeParse(values);
  if (!parsed.success) return { fieldError: { field: 'newPassword', key: 'tooShort' } };

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
    if (code === 'unprocessable') return { fieldError: { field: 'newPassword', key: 'tooShort' } };
    return { error: 'unexpected' };
  }
}
