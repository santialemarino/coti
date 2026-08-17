'use server';

import { revalidatePath } from 'next/cache';
import { redirect } from 'next/navigation';

import { LOGIN_ROUTE } from '@/config/routes';
import { clearActiveBranch, setActiveBranch } from '@/lib/auth/branch';
import { endSession } from '@/lib/auth/session';
import { ALL_BRANCHES } from '@/lib/constants/branch';

/*
 * Switching branch re-answers every branch-scoped read in the tree, not only the route the
 * switcher happens to sit on, so the whole layout is revalidated. A request naming a branch
 * the caller does not reach writes nothing and the re-render puts the switcher back on the
 * truth, which is the only way the list goes stale.
 */
export async function selectBranch(branchId: string): Promise<void> {
  if (branchId === ALL_BRANCHES) await clearActiveBranch();
  else await setActiveBranch(branchId);
  revalidatePath('/', 'layout');
}

// Signing out revokes the session on the API as well as clearing the cookies, so
// the tokens the browser was holding stop working everywhere rather than only here.
export async function signOut(): Promise<void> {
  await endSession();
  redirect(LOGIN_ROUTE);
}
