import { describe, expect, it } from 'vitest';

import { groupQueue } from '@/app/(protected)/rfqs/_components/rfq-queue-groups';
import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

function order(
  id: string,
  status: RfqStatus,
  { archived = false, saleComplete = false }: { archived?: boolean; saleComplete?: boolean } = {},
): RfqRecord {
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
    status,
    needsFollowup: false,
    followupFlaggedAt: null,
    paidAt: null,
    invoiceWaivedAt: null,
    invoiced: false,
    invoicePending: false,
    saleComplete,
    archived,
  };
}

describe('groupQueue', () => {
  // Decision 75: what waits on the seller, the open sales, what waits on others, the closed outcomes.
  it('orders the stacks by who has to act, whatever order the list came in', () => {
    const groups = groupQueue([
      order('rejected', 'REJECTED'),
      order('done', 'ACCEPTED', { saleComplete: true }),
      order('received', 'RECEIVED'),
      order('sent', 'SENT'),
      order('open', 'ACCEPTED'),
      order('quoted', 'QUOTED'),
      order('generated', 'GENERATED'),
      order('failed', 'FAILED'),
      order('change', 'CHANGE_REQUESTED'),
    ]);

    expect(groups.map((group) => group.stack)).toEqual([
      'CHANGE_REQUESTED',
      'FAILED',
      'GENERATED',
      'QUOTED',
      'TO_CLOSE',
      'SENT',
      'RECEIVED',
      'COMPLETED',
      'REJECTED',
    ]);
  });

  it('splits accepted orders by whether the sale is complete', () => {
    const groups = groupQueue([
      order('a', 'ACCEPTED', { saleComplete: true }),
      order('b', 'ACCEPTED'),
      order('c', 'ACCEPTED', { saleComplete: true }),
    ]);

    expect(groups.map((group) => [group.stack, group.records.map((record) => record.id)])).toEqual([
      ['TO_CLOSE', ['b']],
      ['COMPLETED', ['a', 'c']],
    ]);
  });

  // The list arrives follow-ups first, then newest; the top of each stack must stay that one.
  it('keeps the list order inside a group', () => {
    const [group] = groupQueue([order('zeta', 'SENT'), order('alfa', 'SENT')]);

    expect(group?.records.map((record) => record.id)).toEqual(['zeta', 'alfa']);
  });

  it('leaves archived orders out, and with them a group they would be alone in', () => {
    const groups = groupQueue([order('a', 'SENT'), order('b', 'REJECTED', { archived: true })]);

    expect(groups.map((group) => group.stack)).toEqual(['SENT']);
  });
});
