'use server';

import { changePasswordSchema } from '@/app/(protected)/settings/password/form-schema';
import { apiRequest, errorCodeOf } from '@/lib/api/client';
import { getSession, startSession } from '@/lib/auth/session';

export type ChangePasswordErrorKey =
  | 'mismatch'
  | 'tooShort'
  | 'wrongCurrentPassword'
  | 'sessionExpired'
  | 'unexpected';

export interface ChangePasswordState {
  done?: boolean;
  error?: ChangePasswordErrorKey;
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
export async function changePassword(
  _previous: ChangePasswordState,
  formData: FormData,
): Promise<ChangePasswordState> {
  const values = {
    currentPassword: String(formData.get('currentPassword') ?? ''),
    newPassword: String(formData.get('newPassword') ?? ''),
    confirmPassword: String(formData.get('confirmPassword') ?? ''),
  };
  const parsed = changePasswordSchema.safeParse(values);
  if (!parsed.success) {
    return { error: values.newPassword === values.confirmPassword ? 'tooShort' : 'mismatch' };
  }

  try {
    const tokens = await apiRequest<TokenPairRaw>({
      path: '/v1/auth/change-password',
      method: 'POST',
      body: {
        current_password: parsed.data.currentPassword,
        new_password: parsed.data.newPassword,
      },
    });
    await startSession({
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token,
    });
    return { done: true };
  } catch (error) {
    const code = errorCodeOf(error);
    if (code === 'unprocessable') return { error: 'tooShort' };
    if (code === 'unauthenticated') {
      // The route answers 401 for a wrong current password and for a bearer the API
      // no longer honours; telling a user their password is wrong when their session
      // simply lapsed sends them chasing the wrong problem.
      const stillSignedIn = await getSession();
      return { error: stillSignedIn ? 'wrongCurrentPassword' : 'sessionExpired' };
    }
    return { error: 'unexpected' };
  }
}
