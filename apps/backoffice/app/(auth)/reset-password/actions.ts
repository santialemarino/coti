'use server';

import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export interface ResetPasswordResult {
  done?: boolean;
  error?: 'invalidLink' | 'unexpected';
  /* The API's own floor, which is the only field-level rejection this route answers. */
  fieldError?: { field: 'newPassword' };
}

export async function resetPassword(values: ResetPasswordValues): Promise<ResetPasswordResult> {
  // The form validated this already, so a failure here means the request did not come from it.
  const parsed = resetPasswordSchema().safeParse(values);
  if (!parsed.success) return { error: 'unexpected' };

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
    if (code === 'unprocessable') return { fieldError: { field: 'newPassword' } };
    return { error: 'unexpected' };
  }
}
