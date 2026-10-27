import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
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
    paidAt: null,
    invoiceWaivedAt: null,
    invoiced: false,
    invoicePending: false,
    saleComplete: false,
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
  return screen.getByRole('group', { name: new RegExp(copy.status[status]) });
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
    expect(top.textContent).toContain(copy.list.groups.stackHint.replace('{count}', '4'));
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

  // Folding the open order's group and then opening another order of it must not bury the new one.
  it('unfolds the group again when another order of it opens', () => {
    const view = renderList('s1');
    fireEvent.click(within(group('SENT')).getByRole('button', { name: copy.list.groups.collapse }));

    view.rerender(
      <NextIntlClientProvider locale="es" messages={messages}>
        <RfqSidebarList records={[...SENT, QUOTED]} activeRfqId="s3" />
      </NextIntlClientProvider>,
    );

    expect(reachable('SENT')).toHaveLength(4);
    expect(screen.getByRole('button', { current: 'page' }).textContent).toContain('#13');
  });
});

describe('RfqSidebarList closing', () => {
  const PAID_AT = '2026-10-10T12:00:00.000Z';
  const sale = (id: string, quoteNumber: number, facts: Partial<RfqRecord> = {}): RfqRecord => ({
    ...order(id, quoteNumber, 'ACCEPTED'),
    quoteId: `quote-${id}`,
    ...facts,
  });
  const stacks = copy.list.groups.stacks;
  const stackGroup = (name: string) => screen.getByRole('group', { name: new RegExp(`^${name}`) });

  function renderClosing(records: RfqRecord[], invoicingEnabled = true) {
    return render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <RfqListProvider
          hasQueue
          records={records}
          activeBranchId={null}
          userName="Ana"
          userId="me"
          isAdmin
          invoicingEnabled={invoicingEnabled}
        >
          <RfqSidebarList records={records} activeRfqId={null} />
        </RfqListProvider>
      </NextIntlClientProvider>,
    );
  }

  const step = (action: string, reference: string) =>
    screen.queryByRole('button', {
      name: copy.list.closing.labelled.replace('{action}', action).replace('{id}', reference),
    });

  // The closed outcomes sit folded at the foot, heading and count only, until they are shown.
  it('folds «Completados» and the rejected away until they are shown', () => {
    renderClosing([
      sale('done', 31, { paidAt: PAID_AT, saleComplete: true }),
      order('lost', 32, 'REJECTED'),
      order('sent', 33, 'SENT'),
    ]);

    const headings = screen.getAllByRole('heading').map((heading) => heading.textContent);
    expect(headings).toEqual([
      `${copy.status.SENT}1`,
      `${stacks.COMPLETED}1`,
      `${copy.status.REJECTED}1`,
    ]);
    expect(within(stackGroup(stacks.COMPLETED)).queryAllByRole('listitem')).toHaveLength(0);
    expect(within(stackGroup(copy.status.REJECTED)).queryAllByRole('listitem')).toHaveLength(0);

    fireEvent.click(
      within(stackGroup(stacks.COMPLETED)).getByRole('button', { name: copy.list.groups.show }),
    );
    expect(within(stackGroup(stacks.COMPLETED)).getAllByRole('listitem')).toHaveLength(1);
    expect(
      within(stackGroup(stacks.COMPLETED)).getByRole('button', { name: copy.list.groups.hide }),
    ).toBeTruthy();
  });

  it('offers an unpaid sale its payment, and opens the confirmation without opening the order', async () => {
    renderClosing([sale('open', 41)], false);

    const markPaid = step(copy.list.closing.MARK_PAID, '#41') as HTMLElement;
    expect(within(stackGroup(stacks.TO_CLOSE)).getByText(copy.list.closing.MARK_PAID)).toBeTruthy();
    fireEvent.click(markPaid);

    expect(router.push).not.toHaveBeenCalled();
    const dialog = within(await screen.findByRole('dialog'));
    expect(
      dialog.getByText(copy.detail.closure.dialog.title.replace('{reference}', '#41')),
    ).toBeTruthy();
  });

  // The rest of the card keeps opening the order, the step beside it is a target of its own.
  it('still opens the order from the rest of the card', () => {
    renderClosing([sale('open', 41)], false);

    const card = within(stackGroup(stacks.TO_CLOSE))
      .getAllByRole('button')
      .find((button) => button.closest('li') && button.textContent?.includes('#41'));
    fireEvent.click(card as HTMLElement);
    expect(router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('open'));
  });

  it('offers a paid sale its invoice while the account invoices, and nothing once it does not', () => {
    const view = renderClosing([sale('paid', 42, { paidAt: PAID_AT })]);
    expect(step(copy.list.closing.INVOICE, '#42')).toBeTruthy();
    view.unmount();

    renderClosing([sale('paid', 42, { paidAt: PAID_AT, saleComplete: true })], false);
    expect(step(copy.list.closing.INVOICE, '#42')).toBeNull();
    expect(step(copy.list.closing.MARK_PAID, '#42')).toBeNull();
  });

  it.each([
    ['waived', { paidAt: PAID_AT, invoiceWaivedAt: PAID_AT, saleComplete: true }],
    ['invoice pending in ARCA', { paidAt: PAID_AT, invoicePending: true }],
  ])('offers no step on a sale %s', (_, facts) => {
    renderClosing([sale('s', 43, facts)]);

    expect(step(copy.list.closing.INVOICE, '#43')).toBeNull();
    expect(step(copy.list.closing.MARK_PAID, '#43')).toBeNull();
  });

  it('offers no step on any other status', () => {
    renderClosing([{ ...order('q', 44, 'QUOTED'), quoteId: 'quote-q' }]);

    expect(screen.queryByText(copy.list.closing.MARK_PAID)).toBeNull();
    expect(screen.queryByText(copy.list.closing.INVOICE)).toBeNull();
  });
});
