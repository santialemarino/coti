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

  // Below lg the sections are a tab bar; the chosen one carries the page mark like the header row.
  it('renders the tab bar with the same sections and the same mark', () => {
    navigation.pathname = ROUTES.rfqsDetail('r1');
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <PrimaryNav layout="tabs" />
      </NextIntlClientProvider>,
    );

    const tabs = screen.getAllByRole('link');
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      messages.common.nav.orders,
      messages.common.nav.clients,
      messages.common.nav.reports,
      messages.common.nav.administration,
    ]);
    expect(tabs[0]?.getAttribute('aria-current')).toBe('page');
    expect(tabs[1]?.getAttribute('aria-current')).toBeNull();
  });

  it.each([
    [ROUTES.clients, messages.common.nav.clients],
    [ROUTES.reports, messages.common.nav.reports],
    [ROUTES.administration, messages.common.nav.administration],
  ])('marks the tab of the section on %s', (path, label) => {
    navigation.pathname = path;
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <PrimaryNav layout="tabs" />
      </NextIntlClientProvider>,
    );

    expect(screen.getByRole('link', { current: 'page' }).textContent).toBe(label);
  });
});
