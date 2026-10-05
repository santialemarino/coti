import { act, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqQueueColumn } from '@/app/(protected)/rfqs/_components/rfq-queue-column';
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

function tree(hasQueue: boolean) {
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
        <RfqQueueColumn />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function renderAt(pathname: string, hasQueue = true) {
  navigation.pathname = pathname;
  const view = render(tree(hasQueue));
  return {
    column: () => view.container.firstElementChild as HTMLElement,
    navigate: (next: string) => {
      navigation.pathname = next;
      view.rerender(tree(hasQueue));
    },
  };
}

beforeEach(() => vi.clearAllMocks());

describe('RfqQueueColumn', () => {
  it.each([ROUTES.home, ROUTES.rfqsDetail('r1')])('is open on the queue screen %s', (path) => {
    const { column } = renderAt(path);

    expect(column().dataset.open).toBe('true');
    expect(column().hasAttribute('inert')).toBe(false);
  });

  // The table lists the same orders at full width; a rail beside it would repeat it, narrower.
  it.each([ROUTES.rfqs, ROUTES.clients, ROUTES.accountSettings])('is closed on %s', (path) => {
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
});
