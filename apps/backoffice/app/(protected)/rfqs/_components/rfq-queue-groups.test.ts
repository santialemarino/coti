import { describe, expect, it } from 'vitest';

import { FOLDED_STACKS, groupQueue } from '@/app/(protected)/rfqs/_components/rfq-queue-groups';
import type { CustomerReply, RfqRecord, RfqStatus } from '@/lib/api/rfqs';

interface OrderFacts {
  archived?: boolean;
  saleComplete?: boolean;
  acceptanceUnconfirmed?: boolean;
  customerReply?: CustomerReply | null;
}

const QUESTION: CustomerReply = {
  reason: 'CUSTOMER_QUESTION',
  replyAcceptance: null,
  createdAt: '2026-10-02T12:00:00.000Z',
};

function order(
  id: string,
  status: RfqStatus,
  {
    archived = false,
    saleComplete = false,
    acceptanceUnconfirmed = false,
    customerReply = null,
  }: OrderFacts = {},
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
    acceptanceUnconfirmed,
    customerReply,
  };
}

describe('groupQueue', () => {
  /*
   * Who has to act: an acceptance to confirm, what else waits on the seller, the replies to read,
   * the open sales, what waits on others, the closed outcomes.
   */
  it('orders the stacks by who has to act, whatever order the list came in', () => {
    const groups = groupQueue([
      order('wrote', 'SENT', { customerReply: QUESTION }),
      order('to-confirm', 'ACCEPTED', { acceptanceUnconfirmed: true }),
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
      'ACCEPTANCE_TO_CONFIRM',
      'CHANGE_REQUESTED',
      'FAILED',
      'GENERATED',
      'QUOTED',
      'CUSTOMER_WROTE',
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

  it('takes an acceptance to confirm out of the open sales, whatever its closure', () => {
    const groups = groupQueue([
      order('open', 'ACCEPTED'),
      order('confirm', 'ACCEPTED', { acceptanceUnconfirmed: true, saleComplete: true }),
    ]);

    expect(groups.map((group) => [group.stack, group.records.map((record) => record.id)])).toEqual([
      ['ACCEPTANCE_TO_CONFIRM', ['confirm']],
      ['TO_CLOSE', ['open']],
    ]);
  });

  // A reply is only news on a quote still out for an answer; elsewhere the status decides.
  it('moves a sent order with an unread reply to its own stack, and nothing else', () => {
    const groups = groupQueue([
      order('quiet', 'SENT'),
      order('wrote', 'SENT', { customerReply: QUESTION }),
      order('change', 'CHANGE_REQUESTED', { customerReply: QUESTION }),
      order('archived', 'SENT', { customerReply: QUESTION, archived: true }),
    ]);

    expect(groups.map((group) => [group.stack, group.records.map((record) => record.id)])).toEqual([
      ['CHANGE_REQUESTED', ['change']],
      ['CUSTOMER_WROTE', ['wrote']],
      ['SENT', ['quiet']],
      ['ARCHIVED', ['archived']],
    ]);
  });

  // The list arrives follow-ups first, then newest; the top of each stack must stay that one.
  it('keeps the list order inside a group', () => {
    const [group] = groupQueue([order('zeta', 'SENT'), order('alfa', 'SENT')]);

    expect(group?.records.map((record) => record.id)).toEqual(['zeta', 'alfa']);
  });

  it('puts archived orders last in a separate folded stack', () => {
    const groups = groupQueue([order('a', 'SENT'), order('b', 'REJECTED', { archived: true })]);

    expect(groups.map((group) => group.stack)).toEqual(['SENT', 'ARCHIVED']);
    expect(FOLDED_STACKS.has('ARCHIVED')).toBe(true);
  });
});
