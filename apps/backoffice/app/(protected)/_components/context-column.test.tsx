import { act, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ContextColumn } from '@/app/(protected)/_components/context-column';
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
vi.mock('@/lib/api/rfqs-client', () => ({ fetchQueue: vi.fn(async () => []) }));

const { fetchQueue } = await import('@/lib/api/rfqs-client');

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

function tree(hasQueue: boolean, settingsNav = SETTINGS_NAV) {
  return (
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqListProvider
        records={[ORDER]}
        hasQueue={hasQueue}
        activeBranchId={null}
        userName="Ana"
        userId="u1"
        isAdmin
      >
        <ContextColumn settingsNav={settingsNav} />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function renderAt(pathname: string, hasQueue = true, settingsNav = SETTINGS_NAV) {
  navigation.pathname = pathname;
  const view = render(tree(hasQueue, settingsNav));
  return {
    column: () => view.container.firstElementChild as HTMLElement,
    navigate: (next: string) => {
      navigation.pathname = next;
      view.rerender(tree(hasQueue, settingsNav));
    },
  };
}

beforeEach(() => vi.clearAllMocks());

describe('ContextColumn', () => {
  it.each([ROUTES.home, ROUTES.rfqsDetail('r1')])('is open on the queue screen %s', (path) => {
    const { column } = renderAt(path);

    expect(column().dataset.open).toBe('true');
    expect(column().hasAttribute('inert')).toBe(false);
  });

  // The table lists the same orders at full width; a rail beside it would repeat it, narrower.
  it.each([ROUTES.rfqs, ROUTES.clients, ROUTES.reports])('is closed on %s', (path) => {
    const { column } = renderAt(path);

    expect(column().dataset.open).toBe('false');
    expect(column().hasAttribute('inert')).toBe(true);
  });

  it('stays closed for a caller with no queue, even on the queue screens', () => {
    const { column } = renderAt(ROUTES.home, false);

    expect(column().dataset.open).toBe('false');
  });

  it('marks the order the path has open', () => {
    renderAt(ROUTES.rfqsDetail('r1'));

    expect(screen.getByRole('button', { current: 'page' }).textContent).toContain('#07');
  });

  // Losing the mark the moment the path changes would blank the row while the column slides away.
  it('keeps the order marked while the column closes', () => {
    const { navigate } = renderAt(ROUTES.rfqsDetail('r1'));

    navigate(ROUTES.clients);

    expect(screen.getByRole('button', { current: 'page', hidden: true }).textContent).toContain(
      '#07',
    );
  });

  // A layout outlives navigation, so coming back to the queue is the moment its list goes stale.
  // The shell's list is read once; staying within the orders must still bring new ones in.
  it('reads the queue again on every move within the orders, not elsewhere or at first', async () => {
    const { navigate } = renderAt(ROUTES.home);
    expect(fetchQueue).not.toHaveBeenCalled();

    await act(async () => navigate(ROUTES.rfqsDetail('r1')));
    await act(async () => navigate(ROUTES.rfqs));
    expect(fetchQueue).toHaveBeenCalledTimes(2);

    await act(async () => navigate(ROUTES.clients));
    expect(fetchQueue).toHaveBeenCalledTimes(2);

    await act(async () => navigate(ROUTES.home));
    expect(fetchQueue).toHaveBeenCalledTimes(3);
  });

  it('reads the queue again when the window regains focus, only within the orders', async () => {
    const { navigate } = renderAt(ROUTES.home);

    await act(async () => fireEvent.focus(window));
    expect(fetchQueue).toHaveBeenCalledTimes(1);

    navigate(ROUTES.clients);
    await act(async () => fireEvent.focus(window));
    expect(fetchQueue).toHaveBeenCalledTimes(1);
  });

  it('opens on the settings pages with their sections, the queue kept but hidden', () => {
    const { column } = renderAt(ROUTES.branchSettings);

    expect(column().dataset.open).toBe('true');
    expect(screen.getByRole('link', { name: 'Sucursales' }).getAttribute('aria-current')).toBe(
      'page',
    );
    expect(
      screen
        .getByRole('navigation', { name: messages.rfqs.list.title, hidden: true })
        .closest('.hidden'),
    ).not.toBeNull();
  });

  // Visiting settings is not coming back to the queue; only the queue showing again reads it.
  it('does not read the queue again for a move into settings', async () => {
    const { navigate } = renderAt(ROUTES.home);

    await act(async () => navigate(ROUTES.accountSettings));
    expect(fetchQueue).not.toHaveBeenCalled();

    await act(async () => navigate(ROUTES.home));
    expect(fetchQueue).toHaveBeenCalledTimes(1);
  });

  // Below lg a section's landing is its list, and what the list opens takes the page instead.
  it.each([ROUTES.home, ROUTES.settings])('is the page itself below lg on %s', (path) => {
    const { column } = renderAt(path);

    expect(column().dataset.open).toBe('true');
    expect(column().className).toContain('max-lg:[--column-width:100%]');
    expect(column().className).not.toContain('max-lg:hidden');
    expect(column().dataset.root).toBe('true');
  });

  it.each([ROUTES.rfqsDetail('r1'), ROUTES.accountSettings])(
    'steps aside below lg on %s',
    (path) => {
      const { column } = renderAt(path);

      expect(column().dataset.open).toBe('true');
      expect(column().className).toContain('max-lg:hidden');
      expect(column().dataset.root).toBe('false');
    },
  );

  // On a narrow screen the list is the landing, so it carries the first action the landing had.
  it('offers creating an order from the list header, below lg only', () => {
    renderAt(ROUTES.home);

    const create = screen.getByRole('button', { name: messages.rfqs.list.create });
    expect(create.className).toContain('lg:hidden');
  });

  // A seller is offered no sections; their own settings pages must not open an empty column.
  it('stays closed on the settings pages for a caller with no sections', () => {
    const { column } = renderAt(ROUTES.changePassword, true, []);

    expect(column().dataset.open).toBe('false');
  });

  it('keeps the settings sections on show while the column closes', () => {
    const { column, navigate } = renderAt(ROUTES.accountSettings);

    navigate(ROUTES.clients);

    expect(column().dataset.open).toBe('false');
    expect(
      screen
        .getByRole('navigation', { name: messages.settings.title, hidden: true })
        .closest('.hidden'),
    ).toBeNull();
  });
});
