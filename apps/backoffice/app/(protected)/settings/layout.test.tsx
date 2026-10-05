import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const navigation = vi.hoisted(() => ({ pathname: '/settings/account' }));

vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { getSession } = await import('@/lib/auth/session');
const { default: SettingsLayout } = await import('@/app/(protected)/settings/layout');

async function renderAs(role: string, pathname: string) {
  navigation.pathname = pathname;
  vi.mocked(getSession).mockResolvedValue({ role } as never);
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await SettingsLayout({ children: <p>pantalla</p> })}
    </NextIntlClientProvider>,
  );
  return screen.queryByRole('link', { name: messages.settings.backToSections });
}

beforeEach(() => vi.clearAllMocks());

describe('SettingsLayout', () => {
  // Below lg a section is opened from the index, so it names the way back to it.
  it('leads a section back to the index, below lg only', async () => {
    const back = await renderAs('ADMIN', ROUTES.accountSettings);

    expect(back?.getAttribute('href')).toBe(ROUTES.settings);
    expect(back?.classList.contains('lg:hidden')).toBe(true);
  });

  it('offers no way back on the index itself', async () => {
    expect(await renderAs('ADMIN', ROUTES.settings)).toBeNull();
  });

  it('offers a seller, who has no index, no way back', async () => {
    expect(await renderAs('SELLER', ROUTES.changePassword)).toBeNull();
  });

  // The caller's own pages are opened from the profile menu; the list they would go back to lacks them.
  it.each([ROUTES.changePassword, ROUTES.emailSettings])(
    'offers no way back to the sections from %s',
    async (path) => {
      expect(await renderAs('ADMIN', path)).toBeNull();
    },
  );
});
