'use server';

import { revalidatePath } from 'next/cache';

import { userSchema, type UserValues } from '@/app/(protected)/settings/users/form-schema';
import { apiRequest, errorCodeOf, type ApiErrorCode } from '@/lib/api/client';

export type UserErrorKey =
  | 'emailTaken'
  | 'self'
  | 'deactivated'
  | 'notFound'
  | 'rateLimited'
  | 'unauthorized'
  | 'invalid'
  | 'unexpected';

export interface UserResult {
  ok?: true;
  error?: UserErrorKey;
}

/* What reactivating has to replace the record with, which is whatever the row already holds. */
export interface ReactivatableUser {
  id: string;
  name: string;
  email: string;
  role: string;
  branchIds: string[];
}

export async function createUser(values: UserValues): Promise<UserResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = userSchema('create').safeParse(values);
  if (!parsed.success) return { error: 'invalid' };

  return write(
    {
      path: '/v1/users',
      method: 'POST',
      body: { ...profileOf(parsed.data), password: parsed.data.password },
    },
    'invalid',
  );
}

export async function updateUser(userId: string, values: UserValues): Promise<UserResult> {
  const parsed = userSchema('edit').safeParse(values);
  if (!parsed.success) return { error: 'invalid' };

  return write(
    { path: `/v1/users/${userId}`, method: 'PUT', body: profileOf(parsed.data) },
    'invalid',
  );
}

/*
 * Deactivating revokes the tokens the user already holds, so it is how an account takes someone
 * out of the app rather than merely changing what they can reach.
 */
export async function deactivateUser(userId: string): Promise<UserResult> {
  return write({ path: `/v1/users/${userId}`, method: 'DELETE' }, 'self');
}

/*
 * Reactivating is the same replace as an edit with the flag turned back on, which is why it carries
 * the profile it already has: `PUT` replaces the record and requires all of it.
 */
export async function reactivateUser(user: ReactivatableUser): Promise<UserResult> {
  return write(
    {
      path: `/v1/users/${user.id}`,
      method: 'PUT',
      body: {
        name: user.name,
        email: user.email,
        role: user.role,
        branch_ids: user.branchIds,
        is_active: true,
      },
    },
    'invalid',
  );
}

// The administrator never sees the password: the user gets the same single-use link they would
// have asked for themselves.
export async function sendPasswordReset(userId: string): Promise<UserResult> {
  return write({ path: `/v1/users/${userId}/password-reset`, method: 'POST' }, 'deactivated');
}

function profileOf(values: UserValues) {
  return {
    name: values.name,
    email: values.email,
    role: values.role,
    // Sent even when empty, because the API replaces the whole assignment set: omitting it would
    // read as "leave them alone" and a seller could never be stripped of a branch.
    branch_ids: values.branchIds,
  };
}

interface UserWrite {
  path: string;
  method: 'POST' | 'PUT' | 'DELETE';
  body?: Record<string, unknown>;
}

/*
 * `refusal` is what a 422 means for this route, which differs between them: deactivating answers it
 * only for the caller's own user, mailing a link only for a deactivated one, while creating and
 * editing answer it for a value this form already refuses — so there it means the two validations
 * have drifted apart.
 *
 * Revalidates the whole layout rather than this route: an admin editing their own name changes the
 * shell's account menu as much as the list here, and the shell is not on this route's tree.
 */
async function write(request: UserWrite, refusal: UserErrorKey): Promise<UserResult> {
  try {
    await apiRequest(request);
  } catch (error) {
    const code = errorCodeOf(error);
    return { error: code === 'unprocessable' ? refusal : userErrorFor(code) };
  }
  revalidatePath('/', 'layout');
  return { ok: true };
}

function userErrorFor(code: ApiErrorCode): UserErrorKey {
  if (code === 'conflict') return 'emailTaken';
  if (code === 'notFound') return 'notFound';
  if (code === 'rateLimited') return 'rateLimited';
  if (code === 'forbidden' || code === 'unauthenticated') return 'unauthorized';
  if (code === 'badRequest') return 'invalid';
  return 'unexpected';
}
