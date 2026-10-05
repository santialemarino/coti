import { fireEvent, render, screen, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ContextSheet } from '@/app/(protected)/_components/context-sheet';
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
vi.mock('@/components/brand', () => ({ Brand: () => null }));

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
        <ContextSheet settingsNav={SETTINGS_NAV} />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function openAt(pathname: string, records = [ORDER]) {
  navigation.pathname = pathname;
  const view = render(tree(records));
  fireEvent.click(screen.getByRole('button', { name: messages.common.nav.openMenu }));
  return {
    sheet: () => screen.queryByRole('dialog', { name: messages.common.nav.menu }),
    navigate: (next: string) => {
      navigation.pathname = next;
      view.rerender(tree(records));
    },
  };
}

beforeEach(() => vi.clearAllMocks());

describe('ContextSheet', () => {
  it('opens on the main navigation with the queue under it', () => {
    const { sheet } = openAt(ROUTES.home);

    const panel = within(sheet() as HTMLElement);
    expect(panel.getByRole('link', { name: messages.common.nav.orders })).toBeTruthy();
    expect(panel.getByRole('navigation', { name: messages.rfqs.list.title })).toBeTruthy();
    expect(panel.getByRole('button', { name: /#07/ })).toBeTruthy();
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

  // A section without a column has only the main navigation to offer.
  it('holds only the main navigation elsewhere', () => {
    const { sheet } = openAt(ROUTES.clients);

    const panel = within(sheet() as HTMLElement);
    expect(panel.getAllByRole('navigation')).toHaveLength(1);
  });

  // The order already open changes no path when pressed again, so the press itself closes it.
  it('closes on a press that opens an order, the one already open included', () => {
    const { sheet } = openAt(ROUTES.rfqsDetail('r1'));

    fireEvent.click(within(sheet() as HTMLElement).getByRole('button', { current: 'page' }));

    expect(navigation.router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('r1'));
    expect(sheet()).toBeNull();
  });

  it('closes on a press on a link', () => {
    const { sheet } = openAt(ROUTES.home);

    fireEvent.click(
      within(sheet() as HTMLElement).getByRole('link', { name: messages.common.nav.clients }),
    );

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

    navigate(ROUTES.rfqs);

    expect(sheet()).toBeNull();
  });
});
