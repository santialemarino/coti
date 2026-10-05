import { describe, expect, it } from 'vitest';

import { groupQueue } from '@/app/(protected)/rfqs/_components/rfq-queue-groups';
import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

function order(id: string, status: RfqStatus, archived = false): RfqRecord {
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
    archived,
  };
}

describe('groupQueue', () => {
  it('orders the groups the way the workflow moves, whatever order the list came in', () => {
    const groups = groupQueue([order('a', 'ACCEPTED'), order('b', 'SENT'), order('c', 'RECEIVED')]);

    expect(groups.map((group) => group.status)).toEqual(['RECEIVED', 'SENT', 'ACCEPTED']);
  });

  // The list arrives follow-ups first, then newest; the top of each stack must stay that one.
  it('keeps the list order inside a group', () => {
    const [group] = groupQueue([order('zeta', 'SENT'), order('alfa', 'SENT')]);

    expect(group?.records.map((record) => record.id)).toEqual(['zeta', 'alfa']);
  });

  it('leaves archived orders out, and with them a group they would be alone in', () => {
    const groups = groupQueue([order('a', 'SENT'), order('b', 'REJECTED', true)]);

    expect(groups.map((group) => group.status)).toEqual(['SENT']);
  });
});
