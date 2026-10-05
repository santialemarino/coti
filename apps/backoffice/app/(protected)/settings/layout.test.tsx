import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ usePathname: () => '/settings/account' }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('@/lib/api/onboarding', () => ({
  getOnboarding: vi.fn(async () => ({ status: 'COMPLETED', checklist: [] })),
}));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn(async () => []) }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { getSession } = await import('@/lib/auth/session');
const { default: SettingsLayout } = await import('@/app/(protected)/settings/layout');

async function renderAs(role: string) {
  vi.mocked(getSession).mockResolvedValue({ role } as never);
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await SettingsLayout({ children: <p>pantalla</p> })}
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('SettingsLayout', () => {
  // From lg up the context column lists the sections; below it, they must still be reachable.
  it('lists the sections above the page for narrow screens only', async () => {
    await renderAs('ADMIN');

    const nav = screen.getByRole('navigation', { name: 'title' });
    expect(nav.closest('.lg\\:hidden')).not.toBeNull();
    expect(screen.getByText('pantalla')).toBeTruthy();
  });

  it('offers a seller no section list', async () => {
    await renderAs('SELLER');

    expect(screen.queryByRole('navigation')).toBeNull();
  });
});
