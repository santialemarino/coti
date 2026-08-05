import 'server-only';

import { cookies } from 'next/headers';

import { getBranches } from '@/lib/api/branches';
import { isRemembered } from '@/lib/auth/session';
import { BRANCH_COOKIE, sessionCookieOptions } from '@/lib/auth/tokens';

/*
 * The branch the caller is working in. A cookie because it outlives a navigation and no
 * client code reads it — the shell renders the switcher from the server.
 *
 * Nothing here validates the cookie on read, deliberately: no branch header means
 * account-wide for an admin, so discarding one that looks wrong widens their scope instead
 * of narrowing it. The API checks the branch against the account and the caller's
 * assignments on every request and answers 403, which is the check that matters.
 */
export async function getActiveBranchId(): Promise<string | undefined> {
  // Blank is no selection, not a selection of nothing. A delete leaves an empty-valued marker
  // in the jar for the rest of the request, and a caller falling back with `??` would take it
  // as a real value and find no branch by that name.
  return (await cookies()).get(BRANCH_COOKIE)?.value || undefined;
}

/*
 * Writes only a branch the caller reaches, so the cookie can never name one they never had,
 * and reports false when the request named anything else. The lifetime follows the session's:
 * a branch choice that outlived the session that made it would greet the next user with it.
 */
export async function setActiveBranch(branchId: string): Promise<boolean> {
  const branches = await getBranches();
  if (!branches.some((branch) => branch.id === branchId)) return false;

  const jar = await cookies();
  jar.set(BRANCH_COOKIE, branchId, sessionCookieOptions(await isRemembered()));
  return true;
}

// Back to no selection, which the API reads as account-wide for an admin and the assigned
// set for a seller.
export async function clearActiveBranch(): Promise<void> {
  (await cookies()).delete(BRANCH_COOKIE);
}
