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
const { PrimaryNav } = await import('@/app/(protected)/_components/primary-nav');
const { AppHeader } = await import('@/app/(protected)/_components/app-header');

const SESSION = {
  userId: 'u1',
  accountId: 'a1',
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  emailVerified: true,
  role: 'ADMIN' as const,
  emailVerificationRequired: false,
  mailDelivery: true,
};

// The trigger is read before the menu opens: an open menu hides everything outside it.
async function openMenu(session = SESSION) {
  const view = render(await AppHeader({ session }));
  const trigger = view.getByRole('button', { name: /Ana Gómez/ });
  const triggerText = trigger.textContent ?? '';
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  return Object.assign(view, { triggerText });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getBranches).mockResolvedValue([]);
  vi.mocked(getEffectiveBranchId).mockResolvedValue(undefined);
  vi.mocked(getTranslations).mockResolvedValue(((key: string) => key) as never);
});

describe('AppHeader attention', () => {
  const MISSING_MAILBOX = {
    id: 'b1',
    name: 'Centro',
    address: null,
    email: null,
    defaultExpiryDays: 7,
    isActive: true,
  };

  // Configuración is the way to the fix, so it lands on the screen that has it.
  it('leads an admin with a branch missing its mailbox to Sucursales, marked', async () => {
    vi.mocked(getBranches).mockResolvedValue([MISSING_MAILBOX]);

    const view = await openMenu();

    const settings = await view.findByRole('menuitem', { name: /nav\.settings/ });
    expect(settings.getAttribute('href')).toBe(ROUTES.branchSettings);
    expect(settings.textContent).toContain('setup.attention');
    expect(view.triggerText).toContain('nav.pending');
  });

  it('marks nothing when every branch has its mailbox', async () => {
    vi.mocked(getBranches).mockResolvedValue([{ ...MISSING_MAILBOX, email: 'c@corralon.test' }]);

    const view = await openMenu();

    await view.findByRole('menuitem', { name: 'nav.settings' });
    expect(view.triggerText).not.toContain('nav.pending');
  });

  // Not required here, so a suggestion — and only once mail can bring the link.
  it('offers confirming the address to an unconfirmed caller when mail is delivered', async () => {
    const view = await openMenu({ ...SESSION, emailVerified: false, role: 'SELLER' as never });

    const confirm = await view.findByRole('menuitem', { name: /nav\.confirmEmail/ });
    expect(confirm.getAttribute('href')).toBe(ROUTES.verifyEmail);
  });

  it('offers nothing to confirm while mail only reaches the log', async () => {
    const view = await openMenu({
      ...SESSION,
      emailVerified: false,
      role: 'SELLER' as never,
      mailDelivery: false,
    });

    await view.findByRole('menuitem', { name: 'nav.signOut' });
    expect(view.queryByRole('menuitem', { name: /nav\.confirmEmail/ })).toBeNull();
  });
});

describe('AppHeader account menu', () => {
  it('offers settings and sign out without a separate password entry', async () => {
    const view = render(await AppHeader({ session: SESSION }));

    fireEvent.pointerDown(view.getByRole('button', { name: /Ana Gómez/ }), {
      button: 0,
      ctrlKey: false,
    });

    const settings = await view.findByRole('menuitem', { name: 'nav.settings' });
    expect(settings.getAttribute('href')).toBe(ROUTES.settings);
    expect(view.getByRole('menuitem', { name: 'nav.signOut' })).toBeTruthy();
    expect(view.queryByRole('menuitem', { name: 'nav.changePassword' })).toBeNull();
  });

  it('keeps a single branch visible for an administrator', async () => {
    const branch = {
      id: 'b1',
      name: 'Centro',
      address: null,
      email: null,
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

// A phone has room for the logo, the branch and the avatar; everything else moves or folds away.
describe('AppHeader below lg', () => {
  it('leaves the section links to the tab bar', async () => {
    render(await AppHeader({ session: SESSION }));

    expect(vi.mocked(PrimaryNav)).toHaveBeenCalledWith({ className: 'max-lg:hidden' }, undefined);
  });

  it('keeps only the avatar of the profile', async () => {
    const view = render(await AppHeader({ session: SESSION }));

    const name = view.getByText(SESSION.name).parentElement as HTMLElement;
    expect(name.className).toContain('hidden lg:flex');
  });
});
