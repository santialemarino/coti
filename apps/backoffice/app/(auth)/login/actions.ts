'use server';

import { loginSchema, type LoginValues } from '@/app/(auth)/login/form-schema';
import { safeNextPath } from '@/config/routes';
import { type ApiErrorCode } from '@/lib/api/errors';
import { clientAddress } from '@/lib/auth/client-address';
import { startSession } from '@/lib/auth/session';
import { requestLogin } from '@/lib/auth/tokens';

export interface LoginResult {
  redirectTo?: string;
  /*
   * `UNAUTHENTICATED` deliberately covers a wrong password, an unknown address and a disabled
   * user alike — the API answers all three with the same 401 and one code, and the interface
   * must not undo that.
   */
  error?: ApiErrorCode;
}

export async function login(values: LoginValues, next?: string): Promise<LoginResult> {
  /*
   * Re-validated server-side: the client's schema is a courtesy, not a guarantee. A request that
   * did not come from the form is answered the way a refused credential is, because that is all
   * a caller hand-crafting one is entitled to learn.
   */
  const parsed = loginSchema().safeParse(values);
  if (!parsed.success) return { error: 'UNAUTHENTICATED' };

  const attempt = await requestLogin(
    parsed.data.email,
    parsed.data.password,
    parsed.data.rememberMe,
    await clientAddress(),
  );
  if (!attempt.ok || !attempt.tokens) return { error: attempt.code ?? 'INTERNAL' };

  await startSession(attempt.tokens, parsed.data.rememberMe);
  return { redirectTo: safeNextPath(next) };
}
