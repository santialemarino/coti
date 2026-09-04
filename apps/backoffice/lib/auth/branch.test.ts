import { beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { clearActiveBranch, getActiveBranchId, setActiveBranch } from '@/lib/auth/branch';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ isRemembered: vi.fn() }));

const { cookies } = await import('next/headers');
const { getBranches } = await import('@/lib/api/branches');
const { isRemembered } = await import('@/lib/auth/session');

const VILLA_BOSCH = '11111111-1111-4111-8111-111111111111';
const MORON = '22222222-2222-4222-8222-222222222222';

function jar(initial: Record<string, string> = {}) {
  const fake = cookieJar(initial);
  vi.mocked(cookies).mockResolvedValue(fake as unknown as Awaited<ReturnType<typeof cookies>>);
  return fake;
}

function reachable(...ids: string[]) {
  vi.mocked(getBranches).mockResolvedValue(
    ids.map((id) => ({ id, name: id, address: null, defaultExpiryDays: 7, isActive: true })),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(isRemembered).mockResolvedValue(false);
});

describe('the active branch cookie', () => {
  it('reads back the branch it wrote', async () => {
    const store = jar();
    reachable(VILLA_BOSCH, MORON);

    await expect(setActiveBranch(MORON)).resolves.toBe(true);
    await expect(getActiveBranchId()).resolves.toBe(MORON);
    expect(store.set).toHaveBeenCalledWith(BRANCH_COOKIE, MORON, expect.anything());
  });

  it('reports no selection when nothing was chosen', async () => {
    jar();
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  /*
   * Next implements a delete as a set to `''`, so the entry survives the request blank. A caller
   * falling back with `??` takes that as a real choice and looks up a branch nobody selected,
   * leaving the switcher on its placeholder. Pinned directly as well as through the delete
   * below, because this is the reader's contract and not a property of how the value got there.
   */
  it('reads a blank cookie as no selection, not as a branch named nothing', async () => {
    jar({ [BRANCH_COOKIE]: '' });
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  /*
   * The guard that makes the cookie trustworthy: the writer is the only place a branch is
   * checked against the caller's reach, so a value that never passed here can never be in it.
   */
  it('refuses a branch the caller does not reach, and writes nothing', async () => {
    const store = jar();
    reachable(VILLA_BOSCH);

    await expect(setActiveBranch(MORON)).resolves.toBe(false);
    expect(store.set).not.toHaveBeenCalled();
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  it('leaves an existing selection alone when a refused one is attempted', async () => {
    jar({ [BRANCH_COOKIE]: VILLA_BOSCH });
    reachable(VILLA_BOSCH);

    await expect(setActiveBranch(MORON)).resolves.toBe(false);
    await expect(getActiveBranchId()).resolves.toBe(VILLA_BOSCH);
  });

  /*
   * The assertion in the middle is what keeps this honest: the store must still hold the entry,
   * blank, the way Next leaves it. Without that line a jar that simply drops the key passes, and
   * a double kinder than production is what hides the case above.
   */
  it('clears the selection, leaving the blank entry Next leaves', async () => {
    const store = jar({ [BRANCH_COOKIE]: MORON });

    await clearActiveBranch();
    expect(store.delete).toHaveBeenCalledWith(BRANCH_COOKIE);
    expect(store.get(BRANCH_COOKIE)).toEqual({ name: BRANCH_COOKIE, value: '' });
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  /*
   * Never validated on read. No branch header means account-wide for an admin, so dropping a
   * cookie that looks wrong would widen their scope; the API is what answers 403.
   */
  it('hands back a branch it cannot vouch for rather than dropping it', async () => {
    jar({ [BRANCH_COOKIE]: MORON });
    reachable(VILLA_BOSCH);

    await expect(getActiveBranchId()).resolves.toBe(MORON);
    expect(getBranches).not.toHaveBeenCalled();
  });

  // The choice must not outlive the session that made it, or the next user on this browser
  // starts inside a branch they never picked.
  it('follows the session lifetime, persisting only a remembered one', async () => {
    reachable(VILLA_BOSCH);

    const plain = jar();
    await setActiveBranch(VILLA_BOSCH);
    expect(plain.set.mock.calls[0]?.[2]).not.toHaveProperty('maxAge');

    vi.mocked(isRemembered).mockResolvedValue(true);
    const remembered = jar();
    await setActiveBranch(VILLA_BOSCH);
    expect(remembered.set.mock.calls[0]?.[2]).toHaveProperty('maxAge');
  });
});
