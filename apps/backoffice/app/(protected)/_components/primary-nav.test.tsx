import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { PrimaryNav } from '@/app/(protected)/_components/primary-nav';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const navigation = vi.hoisted(() => ({ pathname: '/' }));

vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));

function ordersLink(pathname: string) {
  navigation.pathname = pathname;
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <PrimaryNav />
    </NextIntlClientProvider>,
  );
  return screen.getByRole('link', { name: messages.common.nav.orders });
}

describe('PrimaryNav', () => {
  it('sends Pedidos to the queue', () => {
    expect(ordersLink(ROUTES.home).getAttribute('href')).toBe(ROUTES.home);
  });

  // The queue, its table and an order are one section; the tab must not go dark between them.
  it.each([ROUTES.home, ROUTES.rfqs, ROUTES.rfqsDetail('r1')])(
    'keeps Pedidos lit on %s',
    (path) => {
      expect(ordersLink(path).getAttribute('aria-current')).toBe('page');
    },
  );

  it('leaves Pedidos unlit in another section', () => {
    expect(ordersLink(ROUTES.clients).getAttribute('aria-current')).toBeNull();
    expect(
      screen.getByRole('link', { name: messages.common.nav.clients }).getAttribute('aria-current'),
    ).toBe('page');
  });

  it('lists the sections in a column for the menu sheet', () => {
    navigation.pathname = ROUTES.home;
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <PrimaryNav layout="stack" />
      </NextIntlClientProvider>,
    );

    expect(screen.getByRole('navigation').className).toContain('flex-col');
  });
});
