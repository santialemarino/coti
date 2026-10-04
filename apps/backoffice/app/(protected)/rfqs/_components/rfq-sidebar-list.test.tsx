import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { ROUTES } from '@/config/routes';
import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock('next/navigation', () => ({ useRouter: () => router }));

const copy = messages.rfqs;

function order(id: string, quoteNumber: number, status: RfqStatus): RfqRecord {
  return {
    id,
    quoteNumber,
    client: '',
    createdAt: '2026-10-01T12:00:00.000Z',
    channel: 'email',
    seller: '',
    sellerId: null,
    branch: 'Morón',
    branchId: 'b1',
    quoteId: null,
    itemCount: 0,
    reviewCount: 0,
    status,
    needsFollowup: false,
    followupFlaggedAt: null,
  };
}

const SENT = [
  order('s1', 11, 'SENT'),
  order('s2', 12, 'SENT'),
  order('s3', 13, 'SENT'),
  order('s4', 14, 'SENT'),
];
const QUOTED = order('q1', 21, 'QUOTED');

function renderList(activeRfqId: string | null = null, records = [...SENT, QUOTED]) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqSidebarList records={records} activeRfqId={activeRfqId} />
    </NextIntlClientProvider>,
  );
}

function group(status: RfqStatus) {
  return screen.getByRole('region', { name: new RegExp(copy.status[status]) });
}

// The cards a seller can reach; the edges peeking behind a stack are inert and hidden.
function reachable(status: RfqStatus) {
  return within(group(status))
    .queryAllByRole('button')
    .filter((button) => button.closest('li'));
}

beforeEach(() => vi.clearAllMocks());

describe('RfqSidebarList', () => {
  it('folds every group into a stack to start with', () => {
    renderList();

    expect(reachable('SENT')).toHaveLength(1);
    expect(
      within(group('SENT'))
        .getByRole('button', { name: copy.list.groups.expand })
        .getAttribute('aria-expanded'),
    ).toBe('false');
  });

  // Only the top card and two edges are drawn; the rest of a long group waits for the unfold.
  it('draws two cards behind the top one, inert and hidden from assistive tech', () => {
    renderList();

    const items = group('SENT').querySelectorAll('li');
    expect(items).toHaveLength(3);
    expect([...items].map((item) => item.hasAttribute('inert'))).toEqual([false, true, true]);
    expect(items[1]?.getAttribute('aria-hidden')).toBe('true');
  });

  // As on a phone, a press on a folded stack opens the stack, not the order on top of it.
  it('unfolds the stack when its top card is pressed, without opening the order', () => {
    renderList();

    const top = reachable('SENT')[0] as HTMLElement;
    expect(top.textContent).toContain('Mostrar los 4 pedidos');
    fireEvent.click(top);

    expect(router.push).not.toHaveBeenCalled();
    expect(reachable('SENT')).toHaveLength(4);
  });

  it('opens an order from an unfolded group, and folds the group back on request', async () => {
    renderList();
    fireEvent.click(within(group('SENT')).getByRole('button', { name: copy.list.groups.expand }));

    fireEvent.click(reachable('SENT')[1] as HTMLElement);
    expect(router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('s2'));

    fireEvent.click(within(group('SENT')).getByRole('button', { name: copy.list.groups.collapse }));
    // The extra cards fade out before they leave.
    await waitFor(() => expect(reachable('SENT')).toHaveLength(1));
  });

  it('gives a lone order no stack and no toggle', () => {
    renderList();

    expect(
      within(group('QUOTED')).queryByRole('button', { name: copy.list.groups.expand }),
    ).toBeNull();
    fireEvent.click(reachable('QUOTED')[0] as HTMLElement);
    expect(router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('q1'));
  });

  // The open order must never be buried, including on the first paint of an order's page.
  it('unfolds the open order’s group from the start', () => {
    renderList('s3');

    expect(reachable('SENT')).toHaveLength(4);
    expect(screen.getByRole('button', { current: 'page' }).textContent).toContain('#13');
  });

  it('unfolds the group an order moves into while it is open', () => {
    const view = renderList('q1');
    expect(reachable('SENT')).toHaveLength(1);

    view.rerender(
      <NextIntlClientProvider locale="es" messages={messages}>
        <RfqSidebarList records={[...SENT, { ...QUOTED, status: 'SENT' }]} activeRfqId="q1" />
      </NextIntlClientProvider>,
    );

    expect(reachable('SENT')).toHaveLength(5);
  });
});
