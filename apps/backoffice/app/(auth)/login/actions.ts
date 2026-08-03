'use server';

import { loginSchema, type LoginValues } from '@/app/(auth)/login/form-schema';
import { safeNextPath } from '@/config/routes';
import { startSession } from '@/lib/auth/session';
import { requestLogin } from '@/lib/auth/tokens';

/*
 * The message key the form renders, and which field it belongs on.
 * `invalidCredentials` deliberately covers a wrong password, an unknown address and a
 * disabled user alike — the API answers all three with the same 401, and the
 * interface must not undo that, so it lands on the form rather than on a field.
 */
export type LoginErrorKey = 'invalidCredentials' | 'locked' | 'unreachable' | 'unexpected';

export interface LoginResult {
  redirectTo?: string;
  error?: LoginErrorKey;
}

export async function login(values: LoginValues, next?: string): Promise<LoginResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = loginSchema().safeParse(values);
  if (!parsed.success) return { error: 'invalidCredentials' };

  const attempt = await requestLogin(
    parsed.data.email,
    parsed.data.password,
    parsed.data.rememberMe,
  );
  if (!attempt.ok || !attempt.tokens) return { error: loginErrorFor(attempt.status) };

  await startSession(attempt.tokens, parsed.data.rememberMe);
  return { redirectTo: safeNextPath(next) };
}

function loginErrorFor(status: number): LoginErrorKey {
  // The lockout is the one rejection worth distinguishing: the caller needs to tell
  // "wrong password" from "stop trying for a while".
  if (status === 429) return 'locked';
  if (status === 401) return 'invalidCredentials';
  if (status === 0) return 'unreachable';
  return 'unexpected';
}
