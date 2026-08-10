'use server';

import {
  changePasswordSchema,
  type ChangePasswordValues,
} from '@/app/(protected)/settings/password/form-schema';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { getSession, isRemembered, startSession } from '@/lib/auth/session';

export type ChangePasswordField = 'currentPassword' | 'newPassword';

export interface ChangePasswordResult {
  done?: boolean;
  error?: ApiErrorCode;
  field?: ChangePasswordField;
}

/* Which field a refusal belongs on. A code absent from the map belongs to the form. */
const FIELD_FOR: Partial<Record<ApiErrorCode, ChangePasswordField>> = {
  UNAUTHENTICATED: 'currentPassword',
  PASSWORD_POLICY: 'newPassword',
};

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
  // The form validated this already, so a failure here means the request did not come from it.
  const parsed = changePasswordSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

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
    /*
     * The route answers 401 for a wrong current password and for a bearer the API no longer
     * honours, and only asking again tells them apart — telling a user their password is wrong
     * when their session simply lapsed sends them chasing the wrong problem.
     */
    if (code === 'UNAUTHENTICATED' && !(await getSession())) return { error: 'SESSION_EXPIRED' };
    return { error: code, field: FIELD_FOR[code] };
  }
}
