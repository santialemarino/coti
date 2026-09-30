import { describe, expect, it } from 'vitest';

import type { RfqListItem } from '@/lib/api/rfqs';
import { mapListItem } from './rfq-queue-provider';

const RAW: RfqListItem = {
  id: '10000000-0000-4000-8000-000000000001',
  quote_number: 12,
  client: null,
  created_at: '2026-09-20T12:00:00.000Z',
  channel: 'whatsapp',
  seller_id: null,
  seller: '',
  branch: 'Villa Bosch',
  branch_id: 'b0000000-0000-4000-8000-000000000001',
  quote_id: '20000000-0000-4000-8000-000000000001',
  item_count: 3,
  review_count: 1,
  total: null,
  status: 'SENT',
  needs_followup: true,
  followup_flagged_at: '2026-09-27T09:00:00.000Z',
  archived_at: null,
};

describe('mapListItem', () => {
  it('maps every field of the list row, the follow-up date included', () => {
    expect(mapListItem(RAW)).toEqual({
      id: RAW.id,
      quoteNumber: 12,
      client: '',
      createdAt: RAW.created_at,
      channel: 'whatsapp',
      seller: '',
      sellerId: null,
      branch: 'Villa Bosch',
      branchId: RAW.branch_id,
      quoteId: RAW.quote_id,
      itemCount: 3,
      reviewCount: 1,
      total: undefined,
      status: 'SENT',
      needsFollowup: true,
      followupFlaggedAt: '2026-09-27T09:00:00.000Z',
      archived: false,
    });
  });
});
