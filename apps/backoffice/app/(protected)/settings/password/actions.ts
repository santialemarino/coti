'use server';

import {
  changePasswordSchema,
  type ChangePasswordValues,
} from '@/app/(protected)/settings/password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';
import { getSession, isRemembered, startSession } from '@/lib/auth/session';

export interface ChangePasswordResult {
  done?: boolean;
  error?: 'sessionExpired' | 'unexpected';
  fieldError?:
    | { field: 'currentPassword'; key: 'wrong' }
    | { field: 'newPassword'; key: 'tooShort' };
}

interface TokenPairRaw {
  access_token: string;
  refresh_token: string;
}

/*
 * Changing the password ends every session the user holds, this one included, so the
 * API hands back a fresh pair and it has to be persisted here — otherwise the caller
 * is logged out by their own change.
 */
export async function changePassword(values: ChangePasswordValues): Promise<ChangePasswordResult> {
  const parsed = changePasswordSchema().safeParse(values);
  if (!parsed.success) return { fieldError: { field: 'newPassword', key: 'tooShort' } };

  try {
    const tokens = await apiRequest<TokenPairRaw>({
      path: '/v1/auth/change-password',
      method: 'POST',
      body: {
        current_password: parsed.data.currentPassword,
        new_password: parsed.data.newPassword,
      },
    });
    await startSession(
      { accessToken: tokens.access_token, refreshToken: tokens.refresh_token },
      await isRemembered(),
    );
    return { done: true };
  } catch (error) {
    const code = errorCodeOf(error);
    if (code === 'unprocessable') return { fieldError: { field: 'newPassword', key: 'tooShort' } };
    if (code === 'unauthenticated') {
      // The route answers 401 for a wrong current password and for a bearer the API
      // no longer honours; telling a user their password is wrong when their session
      // simply lapsed sends them chasing the wrong problem.
      const stillSignedIn = await getSession();
      if (stillSignedIn) return { fieldError: { field: 'currentPassword', key: 'wrong' } };
      return { error: 'sessionExpired' };
    }
    return { error: 'unexpected' };
  }
}
