'use server';

import { redirect } from 'next/navigation';

import { loginSchema } from '@/app/(auth)/login/form-schema';
import { safeNextPath } from '@/config/routes';
import { startSession } from '@/lib/auth/session';
import { requestLogin } from '@/lib/auth/tokens';

// The message key the screen renders. `invalidCredentials` deliberately covers a
// wrong password, an unknown address and a disabled user alike — the API answers
// all three with the same 401, and the interface must not undo that.
export type LoginErrorKey = 'invalidCredentials' | 'locked' | 'unreachable' | 'unexpected';

export interface LoginState {
  error?: LoginErrorKey;
  // React resets a form once its action resolves, so the address is handed back for
  // the field to re-seed itself. The password deliberately is not.
  email?: string;
}

export async function login(_previous: LoginState, formData: FormData): Promise<LoginState> {
  const email = String(formData.get('email') ?? '');
  const parsed = loginSchema.safeParse({ email, password: formData.get('password') });
  if (!parsed.success) return { error: 'invalidCredentials', email };

  const attempt = await requestLogin(parsed.data.email, parsed.data.password);
  if (!attempt.ok || !attempt.tokens) {
    return { error: loginErrorFor(attempt.status), email };
  }

  await startSession(attempt.tokens);
  redirect(safeNextPath(String(formData.get('next') ?? '')));
}

function loginErrorFor(status: number): LoginErrorKey {
  // The lockout is the one rejection worth distinguishing: the caller needs to tell
  // "wrong password" from "stop trying for a while".
  if (status === 429) return 'locked';
  if (status === 401) return 'invalidCredentials';
  if (status === 0) return 'unreachable';
  return 'unexpected';
}
