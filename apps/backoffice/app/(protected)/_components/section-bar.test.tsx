import { fireEvent, render, screen, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { SectionBar } from '@/app/(protected)/_components/section-bar';
import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { ROUTES } from '@/config/routes';
import type { RfqRecord } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

const navigation = vi.hoisted(() => ({
  pathname: '/',
  router: { push: vi.fn(), prefetch: vi.fn() },
}));

vi.mock('next/navigation', () => ({
  usePathname: () => navigation.pathname,
  useRouter: () => navigation.router,
}));

const SETTINGS_NAV = [
  { href: ROUTES.accountSettings, label: 'Cuenta' },
  { href: ROUTES.branchSettings, label: 'Sucursales' },
];

const ORDER: RfqRecord = {
  id: 'r1',
  quoteNumber: 7,
  client: 'Obra Norte',
  createdAt: '2026-10-01T12:00:00.000Z',
  channel: 'email',
  seller: '',
  sellerId: null,
  branch: 'Morón',
  branchId: 'b1',
  quoteId: 'q1',
  itemCount: 2,
  reviewCount: 0,
  status: 'QUOTED',
  needsFollowup: false,
  followupFlaggedAt: null,
};

function tree(records: RfqRecord[]) {
  return (
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqListProvider
        records={records}
        hasQueue
        activeBranchId={null}
        userName="Ana"
        userId="u1"
        isAdmin
      >
        <SectionBar settingsNav={SETTINGS_NAV} />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function renderAt(pathname: string, records = [ORDER]) {
  navigation.pathname = pathname;
  const view = render(tree(records));
  return {
    bar: (name: RegExp) => screen.queryByRole('button', { name, expanded: false }),
    sheet: () => screen.queryByRole('dialog'),
    navigate: (next: string) => {
      navigation.pathname = next;
      view.rerender(tree(records));
    },
  };
}

function openAt(pathname: string, records = [ORDER]) {
  const view = renderAt(pathname, records);
  fireEvent.click(screen.getByRole('button', { expanded: false }));
  return view;
}

beforeEach(() => vi.clearAllMocks());

describe('SectionBar', () => {
  // The bar names what it opens, so the count of open orders sits beside the section.
  it('names the queue with its count of open orders', () => {
    const { bar } = renderAt(ROUTES.home, [ORDER, { ...ORDER, id: 'r2', archived: true }]);

    expect(bar(/^Pedidos\s*1$/)).not.toBeNull();
  });

  it('names the settings section on show', () => {
    const { bar } = renderAt(ROUTES.branchSettings);

    expect(bar(/^Configuración\s*Sucursales$/)).not.toBeNull();
  });

  // A section without a column has nothing to open, so there is no bar to read as broken.
  it('is absent where the section has no column', () => {
    renderAt(ROUTES.clients);

    expect(screen.queryByRole('button')).toBeNull();
  });

  it('opens the queue beneath it, marked open', () => {
    const { sheet } = openAt(ROUTES.home);

    const panel = within(sheet() as HTMLElement);
    expect(panel.getByRole('navigation', { name: messages.rfqs.list.title })).toBeTruthy();
    expect(panel.getByRole('button', { name: /#07/ })).toBeTruthy();
    expect(
      screen.getByRole('button', { name: /^Pedidos/, hidden: true }).getAttribute('aria-expanded'),
    ).toBe('true');
  });

  it('carries the settings sections on the settings pages', () => {
    const { sheet } = openAt(ROUTES.branchSettings);

    const panel = within(sheet() as HTMLElement);
    expect(panel.getByRole('link', { name: 'Sucursales' }).getAttribute('aria-current')).toBe(
      'page',
    );
    expect(
      panel.getByRole('navigation', { name: messages.rfqs.list.title }).closest('.hidden'),
    ).not.toBeNull();
  });

  // The order already open changes no path when pressed again, so the press itself closes it.
  it('closes on a press that opens an order, the one already open included', () => {
    const { sheet } = openAt(ROUTES.rfqsDetail('r1'));

    fireEvent.click(within(sheet() as HTMLElement).getByRole('button', { current: 'page' }));

    expect(navigation.router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('r1'));
    expect(sheet()).toBeNull();
  });

  it('closes on a press on a link', () => {
    const { sheet } = openAt(ROUTES.branchSettings);

    fireEvent.click(within(sheet() as HTMLElement).getByRole('link', { name: 'Cuenta' }));

    expect(sheet()).toBeNull();
  });

  // Unfolding a stack is browsing the list, not choosing from it.
  it('stays open while a stack unfolds, from its top card or its toggle', () => {
    const { sheet } = openAt(ROUTES.home, [ORDER, { ...ORDER, id: 'r2', quoteNumber: 8 }]);

    fireEvent.click(within(sheet() as HTMLElement).getByRole('button', { name: /#0[78]/ }));
    expect(sheet()).not.toBeNull();

    fireEvent.click(
      within(sheet() as HTMLElement).getByRole('button', {
        name: messages.rfqs.list.groups.collapse,
      }),
    );
    expect(sheet()).not.toBeNull();
    expect(navigation.router.push).not.toHaveBeenCalled();
  });

  it('closes when the path changes from outside it', () => {
    const { sheet, navigate } = openAt(ROUTES.home);

    navigate(ROUTES.rfqsDetail('r1'));

    expect(sheet()).toBeNull();
  });
});
