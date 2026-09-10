import 'server-only';

import { cookies } from 'next/headers';

import { getBranches } from '@/lib/api/branches';
import { getSession, isRemembered } from '@/lib/auth/session';
import { BRANCH_COOKIE, sessionCookieOptions } from '@/lib/auth/tokens';
import { SELLER_ROLE } from '@/lib/constants/auth';

/*
 * The branch the caller explicitly chose. A cookie because it outlives a navigation and no
 * client code reads it — the shell renders the switcher from the server.
 *
 * Nothing here validates the cookie on read, deliberately: no branch header means
 * account-wide for an admin, so discarding one that looks wrong widens their scope instead
 * of narrowing it. The API checks the branch against the account and the caller's
 * assignments on every request and answers 403, which is the check that matters.
 *
 * A seller with exactly one reachable branch never gets a branch to choose, so their choice
 * is resolved for them: with no cookie they are pinned to the assigned set anyway, and this
 * makes that set visible (the switcher) and sends X-Branch-Id on branch-scoped reads instead
 * of leaving the header silent. A seller with several keeps picking; an admin changes nothing.
 */
export async function getActiveBranchId(): Promise<string | undefined> {
  // Blank is no selection: a delete leaves the entry empty for the rest of the request.
  const selected = (await cookies()).get(BRANCH_COOKIE)?.value || undefined;
  if (selected) return selected;

  const session = await getSession();
  if (session?.role !== SELLER_ROLE) return undefined;

  const branches = await getBranches();
  return branches.length === 1 ? branches[0]?.id : undefined;
}

export async function getEffectiveBranchId(
  branches: ReadonlyArray<{ id: string }>,
): Promise<string | undefined> {
  return (await getActiveBranchId()) ?? (branches.length === 1 ? branches[0]?.id : undefined);
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
