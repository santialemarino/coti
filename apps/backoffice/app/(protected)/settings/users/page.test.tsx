import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/app/(protected)/settings/users/_components/user-table', () => ({
  UserTable: vi.fn(() => null),
}));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn(), getAccountBranches: vi.fn() }));
vi.mock('@/lib/api/users', () => ({ getUsers: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { UserTable } = await import('@/app/(protected)/settings/users/_components/user-table');
const { getAccountBranches, getBranches } = await import('@/lib/api/branches');
const { getUsers } = await import('@/lib/api/users');
const { requireAdmin } = await import('@/lib/auth/session');
const { getTranslations } = await import('next-intl/server');
const { default: UserSettingsPage } = await import('@/app/(protected)/settings/users/page');

const SESSION = {
  userId: 'u1',
  accountId: 'a1',
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: 'ADMIN',
};
const CENTRAL = {
  id: 'b1',
  name: 'Casa Central',
  address: null,
  defaultExpiryDays: 7,
  isActive: true,
};

async function renderPage() {
  render(await UserSettingsPage());
  return vi.mocked(UserTable).mock.calls[0]?.[0];
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(requireAdmin).mockResolvedValue(SESSION);
  vi.mocked(getTranslations).mockResolvedValue(((key: string) => key) as never);
  vi.mocked(getUsers).mockResolvedValue([]);
  vi.mocked(getBranches).mockResolvedValue([CENTRAL]);
  vi.mocked(getAccountBranches).mockResolvedValue([]);
});

describe('UserSettingsPage', () => {
  // A seller must not reach this at all, and the API is not the only thing that says so: it answers
  // 404 rather than 403 so the screen does not advertise itself.
  it('refuses anyone but an admin before reading anything', async () => {
    await renderPage();

    expect(requireAdmin).toHaveBeenCalledOnce();
  });

  /*
   * The reach list, never the administration one. `ExistAllInAccount` filters on `is_active`, so the
   * API refuses to assign a user to a closed branch — offering one would be a checkbox that can
   * only fail, and the two reads are deliberately separate functions for exactly this reason.
   */
  it('offers the branches a user can actually be assigned to', async () => {
    const props = await renderPage();

    expect(getBranches).toHaveBeenCalledOnce();
    expect(getAccountBranches).not.toHaveBeenCalled();
    expect(props?.branches).toEqual([CENTRAL]);
  });

  // Every self-edit guard in the interface is decided by comparing against this, so the wrong id
  // here would hide the wrong row's actions and leave the caller able to lock themselves out.
  it('tells the listing which user the caller is', async () => {
    const props = await renderPage();

    expect(props?.currentUserId).toBe(SESSION.userId);
  });
});
