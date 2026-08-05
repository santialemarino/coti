'use server';

import { revalidatePath } from 'next/cache';

import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { apiRequest, errorCodeOf, type ApiErrorCode } from '@/lib/api/client';
import { clearActiveBranch, getActiveBranchId } from '@/lib/auth/branch';

export type BranchErrorKey = 'lastActive' | 'notFound' | 'unauthorized' | 'invalid' | 'unexpected';

export interface BranchResult {
  ok?: true;
  error?: BranchErrorKey;
}

export async function createBranch(values: BranchValues): Promise<BranchResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = branchSchema().safeParse(values);
  if (!parsed.success) return { error: 'invalid' };

  return write({ path: '/v1/branches', method: 'POST', body: bodyOf(parsed.data) }, 'invalid');
}

export async function updateBranch(branchId: string, values: BranchValues): Promise<BranchResult> {
  const parsed = branchSchema().safeParse(values);
  if (!parsed.success) return { error: 'invalid' };

  return write(
    { path: `/v1/branches/${branchId}`, method: 'PUT', body: bodyOf(parsed.data) },
    'invalid',
  );
}

/*
 * Closing the branch the caller is scoped to has to drop the selection with it: the API refuses a
 * branch that is not active, so the cookie would answer 403 on every branch-scoped read afterwards
 * and leave them locked out of the app until they noticed the switcher.
 */
export async function closeBranch(branchId: string): Promise<BranchResult> {
  const result = await write({ path: `/v1/branches/${branchId}`, method: 'DELETE' }, 'lastActive');
  if (!result.ok) return result;

  if ((await getActiveBranchId()) === branchId) await clearActiveBranch();
  return result;
}

function bodyOf(values: BranchValues) {
  return {
    name: values.name,
    // Omitted rather than empty: the API's optional fields are pointers with `omitempty`, which
    // only skips a nil one, so an empty string would reach the column.
    address: values.address || undefined,
    default_expiry_days: Number(values.defaultExpiryDays),
  };
}

interface BranchWrite {
  path: string;
  method: 'POST' | 'PUT' | 'DELETE';
  body?: Record<string, unknown>;
}

/*
 * `refusal` is what a 422 means for this route, which is not the same on all of them: closing
 * answers 422 only for the last active branch, while creating and editing answer it for a value
 * this form already refuses — so there it means the two validations have drifted apart.
 *
 * Revalidates the whole layout rather than this route: a branch that opens or closes changes the
 * switcher in the shell as much as the list here, and the shell is not on this route's tree.
 */
async function write(request: BranchWrite, refusal: BranchErrorKey): Promise<BranchResult> {
  try {
    await apiRequest(request);
  } catch (error) {
    const code = errorCodeOf(error);
    return { error: code === 'unprocessable' ? refusal : branchErrorFor(code) };
  }
  revalidatePath('/', 'layout');
  return { ok: true };
}

function branchErrorFor(code: ApiErrorCode): BranchErrorKey {
  if (code === 'notFound') return 'notFound';
  if (code === 'forbidden' || code === 'unauthenticated') return 'unauthorized';
  if (code === 'badRequest') return 'invalid';
  return 'unexpected';
}
