import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/app/(protected)/settings/account/_components/account-form', () => ({
  AccountForm: vi.fn(() => null),
}));
vi.mock('@/lib/api/account', () => ({ getAccount: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { AccountForm } = await import('@/app/(protected)/settings/account/_components/account-form');
const { getAccount } = await import('@/lib/api/account');
const { requireAdmin } = await import('@/lib/auth/session');
const { getTranslations } = await import('next-intl/server');
const { default: AccountSettingsPage } = await import('@/app/(protected)/settings/account/page');

const ACCOUNT = {
  id: 'a1',
  name: 'Corralón San Martín',
  legalName: null,
  taxId: null,
  brandLogoUrl: null,
  brandColor: '#C2410C',
};

async function renderPage() {
  render(await AccountSettingsPage());
  return vi.mocked(AccountForm).mock.calls[0]?.[0];
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(requireAdmin).mockResolvedValue({
    userId: 'u1',
    accountId: 'a1',
    name: 'Ana Gómez',
    email: 'ana@corralon.test',
    emailVerified: true,
    role: 'ADMIN',
  });
  vi.mocked(getTranslations).mockResolvedValue(((key: string) => key) as never);
  vi.mocked(getAccount).mockResolvedValue(ACCOUNT);
});

describe('AccountSettingsPage', () => {
  /*
   * Reading the account is not admin-only on the API — anything naming the corralón needs it — so
   * the gate on this screen is the only thing keeping a seller off the form that writes it.
   */
  it('refuses anyone but an admin', async () => {
    await renderPage();

    expect(requireAdmin).toHaveBeenCalledOnce();
  });

  it('hands the form the record it is editing', async () => {
    const props = await renderPage();

    expect(getAccount).toHaveBeenCalledOnce();
    expect(props?.account).toEqual(ACCOUNT);
  });
});
