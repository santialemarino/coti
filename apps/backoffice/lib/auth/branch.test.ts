import { beforeEach, describe, expect, it, vi } from 'vitest';

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

// The slice of Next's cookie jar this module touches, backed by a real map so a write is
// readable afterwards — the round trip is the thing under test.
function jar(initial: Record<string, string> = {}) {
  const store = new Map(Object.entries(initial));
  const fake = {
    get: (name: string) => (store.has(name) ? { name, value: store.get(name) } : undefined),
    // Rest-typed so a test can read back the options the writer chose, third argument included.
    set: vi.fn((...args: [string, string, { maxAge?: number }?]) => store.set(args[0], args[1])),
    delete: vi.fn((name: string) => store.delete(name)),
  };
  vi.mocked(cookies).mockResolvedValue(fake as unknown as Awaited<ReturnType<typeof cookies>>);
  return { ...fake, store };
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
    const cookieJar = jar();
    reachable(VILLA_BOSCH, MORON);

    await expect(setActiveBranch(MORON)).resolves.toBe(true);
    await expect(getActiveBranchId()).resolves.toBe(MORON);
    expect(cookieJar.set).toHaveBeenCalledWith(BRANCH_COOKIE, MORON, expect.anything());
  });

  it('reports no selection when nothing was chosen', async () => {
    jar();
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  /*
   * A delete leaves an empty-valued marker in the jar for the rest of the request, so a read
   * that followed one in the same render got `''`. Callers fall back with `??`, which takes
   * that as a real choice — the switcher went blank after switching to account-wide.
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
    const cookieJar = jar();
    reachable(VILLA_BOSCH);

    await expect(setActiveBranch(MORON)).resolves.toBe(false);
    expect(cookieJar.set).not.toHaveBeenCalled();
    await expect(getActiveBranchId()).resolves.toBeUndefined();
  });

  it('leaves an existing selection alone when a refused one is attempted', async () => {
    jar({ [BRANCH_COOKIE]: VILLA_BOSCH });
    reachable(VILLA_BOSCH);

    await expect(setActiveBranch(MORON)).resolves.toBe(false);
    await expect(getActiveBranchId()).resolves.toBe(VILLA_BOSCH);
  });

  // Clearing is how a caller gets back to account-wide, so it must remove the cookie rather
  // than blank it: an empty header and an absent one are different asks.
  it('clears the selection', async () => {
    const cookieJar = jar({ [BRANCH_COOKIE]: MORON });

    await clearActiveBranch();
    expect(cookieJar.delete).toHaveBeenCalledWith(BRANCH_COOKIE);
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
