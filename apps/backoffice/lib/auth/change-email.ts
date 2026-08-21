'use server';

import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { changeEmailSchema, type ChangeEmailValues } from '@/lib/auth/change-email-schema';
import { getSession } from '@/lib/auth/session';

export type ChangeEmailField = 'newEmail' | 'currentPassword';

export interface ChangeEmailResult {
  done?: boolean;
  error?: ApiErrorCode;
  field?: ChangeEmailField;
}

/* Which field a refusal belongs on. A code absent from the map belongs to the form. */
const FIELD_FOR: Partial<Record<ApiErrorCode, ChangeEmailField>> = {
  UNAUTHENTICATED: 'currentPassword',
  EMAIL_TAKEN: 'newEmail',
  CONFLICT: 'newEmail',
};

/*
 * The session outlives the change, unlike a password's: the address moves and the confirmation
 * goes with it, so no pair is re-issued and the caller keeps the token that gets them to the
 * screen explaining the new mail. Which is the whole point — the route is reachable while
 * unconfirmed precisely so a mistyped address is recoverable.
 */
export async function changeEmail(values: ChangeEmailValues): Promise<ChangeEmailResult> {
  // The form validated this already, so a failure here means the request did not come from it.
  const parsed = changeEmailSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    await apiRequest({
      path: '/v1/auth/change-email',
      method: 'POST',
      body: { new_email: parsed.data.newEmail, current_password: parsed.data.currentPassword },
    });
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
