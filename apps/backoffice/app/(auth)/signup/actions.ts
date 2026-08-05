'use server';

import { signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest, errorCodeOf } from '@/lib/api/client';
import { startSession } from '@/lib/auth/session';

// The answer also carries the account and the branch, which no screen needs from here. Optional
// so the check below is a real one rather than a cast over a body that never arrived.
interface SignupRaw {
  tokens?: { access_token?: string; refresh_token?: string };
}

export type SignupErrorKey = 'rateLimited' | 'unreachable' | 'unexpected';

export interface SignupResult {
  redirectTo?: string;
  error?: SignupErrorKey;
  /* The field the API's rejection belongs to. The wizard has to show that field's step for it. */
  fieldError?:
    | { field: 'adminEmail'; key: 'emailTaken' }
    | { field: 'adminPassword'; key: 'tooShort' };
}

export async function signup(values: SignupValues): Promise<SignupResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = signupSchema().safeParse(values);
  if (!parsed.success) return { error: 'unexpected' };
  const data = parsed.data;

  // Built outside the try, which is there for a request that fails — a mapping bug caught by
  // it would come back as an API rejection and read as one.
  const body = {
    account_name: data.accountName,
    legal_name: optional(data.legalName),
    tax_id: optional(data.taxId),
    branch_name: data.branchName,
    branch_address: optional(data.branchAddress),
    admin_name: data.adminName,
    admin_email: data.adminEmail,
    admin_password: data.adminPassword,
  };

  let created: SignupRaw;
  try {
    created = await apiRequest<SignupRaw>({
      path: '/v1/public/accounts',
      method: 'POST',
      authenticated: false,
      body,
    });
  } catch (error) {
    return rejectionFor(errorCodeOf(error));
  }

  if (!created.tokens?.access_token || !created.tokens.refresh_token)
    return { error: 'unexpected' };

  // The account exists either way, so a session is opened rather than sending them to log in
  // with a password the API has not confirmed the address for yet.
  await startSession({
    accessToken: created.tokens.access_token,
    refreshToken: created.tokens.refresh_token,
  });
  return { redirectTo: ROUTES.verifyEmail };
}

/*
 * An omitted key becomes NULL; an empty string becomes an empty string. The API's optional
 * fields are pointers with `omitempty`, which passes a non-nil pointer to "" straight through
 * to the column.
 */
function optional(value: string): string | undefined {
  return value || undefined;
}

function rejectionFor(code: ReturnType<typeof errorCodeOf>): SignupResult {
  // The one conflict registration can answer: the address is already registered somewhere,
  // and login resolves a user across every account, so it cannot be reused.
  if (code === 'conflict') return { fieldError: { field: 'adminEmail', key: 'emailTaken' } };
  // Only reachable when the API's own floor sits above the one this form mirrors.
  if (code === 'unprocessable') return { fieldError: { field: 'adminPassword', key: 'tooShort' } };
  if (code === 'rateLimited') return { error: 'rateLimited' };
  if (code === 'unreachable') return { error: 'unreachable' };
  return { error: 'unexpected' };
}
