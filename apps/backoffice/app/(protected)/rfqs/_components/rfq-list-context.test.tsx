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
