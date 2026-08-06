'use server';

import { revalidatePath } from 'next/cache';

import { userSchema, type UserValues } from '@/app/(protected)/settings/users/form-schema';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface UserResult {
  ok?: true;
  error?: ApiErrorCode;
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
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({
    path: '/v1/users',
    method: 'POST',
    body: { ...profileOf(parsed.data), password: parsed.data.password },
  });
}

export async function updateUser(userId: string, values: UserValues): Promise<UserResult> {
  const parsed = userSchema('edit').safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({ path: `/v1/users/${userId}`, method: 'PUT', body: profileOf(parsed.data) });
}

/*
 * Deactivating revokes the tokens the user already holds, so it is how an account takes someone
 * out of the app rather than merely changing what they can reach.
 */
export async function deactivateUser(userId: string): Promise<UserResult> {
  return write({ path: `/v1/users/${userId}`, method: 'DELETE' });
}

/*
 * Reactivating is the same replace as an edit with the flag turned back on, which is why it carries
 * the profile it already has: `PUT` replaces the record and requires all of it.
 */
export async function reactivateUser(user: ReactivatableUser): Promise<UserResult> {
  return write({
    path: `/v1/users/${user.id}`,
    method: 'PUT',
    body: {
      name: user.name,
      email: user.email,
      role: user.role,
      branch_ids: user.branchIds,
      is_active: true,
    },
  });
}

// The administrator never sees the password: the user gets the same single-use link they would
// have asked for themselves.
export async function sendPasswordReset(userId: string): Promise<UserResult> {
  return write({ path: `/v1/users/${userId}/password-reset`, method: 'POST' });
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
 * Revalidates the whole layout rather than this route: an admin editing their own name changes the
 * shell's account menu as much as the list here, and the shell is not on this route's tree.
 */
async function write(request: UserWrite): Promise<UserResult> {
  try {
    await apiRequest(request);
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath('/', 'layout');
  return { ok: true };
}
