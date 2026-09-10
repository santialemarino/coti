import { fireEvent, render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';

vi.mock('@/app/(protected)/_components/branch-switcher', () => ({
  BranchSwitcher: vi.fn(() => null),
}));
vi.mock('@/app/(protected)/_components/primary-nav', () => ({
  PrimaryNav: vi.fn(() => null),
}));
vi.mock('@/app/(protected)/actions', () => ({ signOut: vi.fn() }));
vi.mock('@/components/brand', () => ({ Brand: vi.fn(() => null) }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getEffectiveBranchId: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getBranches } = await import('@/lib/api/branches');
const { getEffectiveBranchId } = await import('@/lib/auth/branch');
const { getTranslations } = await import('next-intl/server');
const { BranchSwitcher } = await import('@/app/(protected)/_components/branch-switcher');
const { AppHeader } = await import('@/app/(protected)/_components/app-header');

const SESSION = {
  userId: 'u1',
  accountId: 'a1',
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  emailVerified: true,
  role: 'ADMIN' as const,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getBranches).mockResolvedValue([]);
  vi.mocked(getEffectiveBranchId).mockResolvedValue(undefined);
  vi.mocked(getTranslations).mockResolvedValue(((key: string) => key) as never);
});

describe('AppHeader account menu', () => {
  it('offers settings and sign out without a separate password entry', async () => {
    const view = render(await AppHeader({ session: SESSION }));

    fireEvent.pointerDown(view.getByRole('button', { name: /Ana Gómez/ }), {
      button: 0,
      ctrlKey: false,
    });

    const settings = await view.findByRole('menuitem', { name: 'nav.settings' });
    expect(settings.getAttribute('href')).toBe(ROUTES.accountSettings);
    expect(view.getByRole('menuitem', { name: 'nav.signOut' })).toBeTruthy();
    expect(view.queryByRole('menuitem', { name: 'nav.changePassword' })).toBeNull();
  });

  it('keeps a single branch visible for an administrator', async () => {
    const branch = {
      id: 'b1',
      name: 'Centro',
      address: null,
      defaultExpiryDays: 7,
      isActive: true,
    };
    vi.mocked(getBranches).mockResolvedValue([branch]);
    vi.mocked(getEffectiveBranchId).mockResolvedValue(branch.id);

    render(await AppHeader({ session: SESSION }));

    expect(vi.mocked(BranchSwitcher)).toHaveBeenCalledWith(
      expect.objectContaining({ branches: [branch], activeBranchId: branch.id, isAdmin: true }),
      undefined,
    );
  });
});
