'use server';

import { revalidatePath } from 'next/cache';

import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { clearActiveBranch, getActiveBranchId } from '@/lib/auth/branch';

export interface BranchResult {
  ok?: true;
  error?: ApiErrorCode;
}

/* What reopening needs to replace the record with, which is whatever the row already holds. */
export interface ReopenableBranch {
  id: string;
  name: string;
  address: string | null;
  defaultExpiryDays: number;
}

export async function createBranch(values: BranchValues): Promise<BranchResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = branchSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({ path: '/v1/branches', method: 'POST', body: bodyOf(parsed.data) });
}

export async function updateBranch(branchId: string, values: BranchValues): Promise<BranchResult> {
  const parsed = branchSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  return write({ path: `/v1/branches/${branchId}`, method: 'PUT', body: bodyOf(parsed.data) });
}

/*
 * Closing the branch the caller is scoped to has to drop the selection with it: the API refuses a
 * branch that is not active, so the cookie would answer 403 on every branch-scoped read afterwards
 * and leave them locked out of the app until they noticed the switcher.
 */
export async function closeBranch(branchId: string): Promise<BranchResult> {
  const result = await write({ path: `/v1/branches/${branchId}`, method: 'DELETE' });
  if (!result.ok) return result;

  if ((await getActiveBranchId()) === branchId) await clearActiveBranch();
  return result;
}

/*
 * Reopening is the same replace as an edit with the flag turned back on, which is why it carries the
 * branch's current name and expiry: `PUT` replaces the record and requires both.
 */
export async function reopenBranch(branch: ReopenableBranch): Promise<BranchResult> {
  return write({
    path: `/v1/branches/${branch.id}`,
    method: 'PUT',
    body: {
      name: branch.name,
      address: branch.address ?? undefined,
      default_expiry_days: branch.defaultExpiryDays,
      is_active: true,
    },
  });
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
 * Revalidates the whole layout rather than this route: a branch that opens or closes changes the
 * switcher in the shell as much as the list here, and the shell is not on this route's tree.
 */
async function write(request: BranchWrite): Promise<BranchResult> {
  try {
    await apiRequest(request);
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath('/', 'layout');
  return { ok: true };
}
