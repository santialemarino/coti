import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider, useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import type { RfqRecord } from '@/lib/api/rfqs';

vi.mock('@/lib/api/rfqs-client', () => ({ fetchQueue: vi.fn() }));

const { fetchQueue } = await import('@/lib/api/rfqs-client');

function order(id: string): RfqRecord {
  return {
    id,
    quoteNumber: null,
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
    status: 'QUOTED',
    needsFollowup: false,
    followupFlaggedAt: null,
    paidAt: null,
    invoiceWaivedAt: null,
    invoiced: false,
    invoicePending: false,
    saleComplete: false,
  };
}

let list: ReturnType<typeof useRfqList>;

function Probe() {
  list = useRfqList();
  return <p>{list.records.map((record) => record.id).join(',')}</p>;
}

function renderList() {
  render(
    <RfqListProvider
      records={[order('a')]}
      hasQueue
      loadFailed={false}
      activeBranchId={null}
      userName="Ana"
      userId="u1"
      isAdmin
    >
      <Probe />
    </RfqListProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('RfqListProvider', () => {
  it('replaces the list with what a re-read brings', async () => {
    vi.mocked(fetchQueue).mockResolvedValue([order('a'), order('b')]);
    renderList();

    await act(() => list.reload());

    expect(screen.getByText('a,b')).toBeTruthy();
  });

  // A failed re-read must not empty the queue the seller is looking at.
  it('keeps the list on screen when a re-read fails', async () => {
    vi.mocked(fetchQueue).mockRejectedValue(new Error('down'));
    renderList();

    await act(() => list.reload());

    expect(screen.getByText('a')).toBeTruthy();
  });

  // A write's patch is newer than a read that was already on its way; the read must not undo it.
  it('drops a re-read that lands after a newer change', async () => {
    let answer: (records: RfqRecord[]) => void = () => undefined;
    vi.mocked(fetchQueue).mockReturnValue(new Promise((resolve) => (answer = resolve)));
    renderList();

    let pending: Promise<void> = Promise.resolve();
    act(() => {
      pending = list.reload();
    });
    act(() => list.updateRecord('a', { client: 'Obra Norte' }));
    await act(async () => {
      answer([order('stale')]);
      await pending;
    });

    expect(screen.getByText('a')).toBeTruthy();
    expect(list.records[0]?.client).toBe('Obra Norte');
  });
});

describe('RfqListProvider invoicing switch', () => {
  function withSwitch(enabled: boolean | null) {
    return (
      <RfqListProvider
        records={[order('a')]}
        hasQueue
        activeBranchId={null}
        userName="Ana"
        userId="u1"
        isAdmin
        invoicingEnabled={enabled}
        invoiceOnPayment={enabled}
      >
        <Probe />
      </RfqListProvider>
    );
  }

  // A failed account read must not take the invoicing steps away; the next read that lands decides.
  it('keeps the last known switch while the account cannot be read', () => {
    const view = render(withSwitch(true));

    view.rerender(withSwitch(null));
    expect(list.invoicingEnabled).toBe(true);
    expect(list.invoiceOnPayment).toBe(true);

    view.rerender(withSwitch(false));
    expect(list.invoicingEnabled).toBe(false);
  });
});

describe('RfqListProvider archive', () => {
  function providerWith(records: RfqRecord[]) {
    return (
      <RfqListProvider
        records={records}
        hasQueue
        activeBranchId={null}
        userName="Ana"
        userId="u1"
        isAdmin
      >
        <Probe />
      </RfqListProvider>
    );
  }

  it('re-reads the open queue only until the archive is asked for', async () => {
    vi.mocked(fetchQueue).mockResolvedValue([order('a')]);
    renderList();

    await act(() => list.reload());

    expect(fetchQueue).toHaveBeenCalledExactlyOnceWith(false);
    expect(list.archivedState).toBe('idle');
  });

  it('brings the archive in once asked, and keeps it on every later re-read', async () => {
    vi.mocked(fetchQueue).mockResolvedValue([order('a'), { ...order('old'), archived: true }]);
    renderList();

    act(() => list.requestArchived());
    expect(list.archivedState).toBe('loading');
    await vi.waitFor(() => expect(list.archivedState).toBe('loaded'));
    expect(screen.getByText('a,old')).toBeTruthy();

    act(() => list.requestArchived());
    await act(() => list.reload());
    expect(vi.mocked(fetchQueue).mock.calls).toEqual([[true], [true]]);
  });

  // The server's list carries no archive, so a refresh would otherwise drop it from the filter.
  it('reads the archive again when the server hands down a fresh list', async () => {
    vi.mocked(fetchQueue).mockResolvedValue([order('a'), { ...order('old'), archived: true }]);
    const view = render(providerWith([order('a')]));
    act(() => list.requestArchived());
    await vi.waitFor(() => expect(list.archivedState).toBe('loaded'));

    view.rerender(providerWith([order('a'), order('b')]));

    await vi.waitFor(() => expect(fetchQueue).toHaveBeenCalledTimes(2));
    expect(fetchQueue).toHaveBeenLastCalledWith(true);
    await vi.waitFor(() => expect(list.archivedState).toBe('loaded'));
  });

  it('reports a refused archive and asks again only on request', async () => {
    vi.mocked(fetchQueue).mockRejectedValueOnce(new Error('down'));
    renderList();

    act(() => list.requestArchived());
    await vi.waitFor(() => expect(list.archivedState).toBe('failed'));
    await act(() => list.reload());
    expect(fetchQueue).toHaveBeenLastCalledWith(false);
  });
});
