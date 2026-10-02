'use server';

import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { endSession } from '@/lib/auth/session';

export interface ResetPasswordResult {
  done?: boolean;
  error?: ApiErrorCode;
  field?: 'newPassword';
}

/*
 * Which field a refusal belongs on. Only the policy is the password's; a link that is unknown,
 * expired or already used belongs to the screen, and so does anything else.
 */
const FIELD_FOR: Partial<Record<ApiErrorCode, 'newPassword'>> = {
  PASSWORD_POLICY: 'newPassword',
};

export async function resetPassword(values: ResetPasswordValues): Promise<ResetPasswordResult> {
  // The form validated this already, so a failure here means the request did not come from it.
  const parsed = resetPasswordSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    await apiRequest({
      path: '/v1/public/auth/reset-password',
      method: 'POST',
      authenticated: false,
      body: { token: parsed.data.token, new_password: parsed.data.newPassword },
    });
  } catch (error) {
    const code = errorCodeOf(error);
    return { error: code, field: FIELD_FOR[code] };
  }
  // The link may have opened in a browser signed in as someone else — an admin trying the invite
  // they sent. Whoever chose this password logs in with it next, so that session goes too.
  await endSession();
  return { done: true };
}
