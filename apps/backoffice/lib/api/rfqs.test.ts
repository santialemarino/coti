import { describe, expect, it } from 'vitest';

import { mapListItem, normalizeRfqStatus, type RfqListItem } from '@/lib/api/rfqs';

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

describe('normalizeRfqStatus', () => {
  /*
   * DRAFT is an internal quote_state, never a business state the seller sees; the wire can still
   * carry it (the backend merges quote.current_status into the RFQ status), so it collapses onto
   * GENERATED at the display boundary.
   */
  it('collapses DRAFT onto GENERATED, case-insensitively', () => {
    expect(normalizeRfqStatus('DRAFT')).toBe('GENERATED');
    expect(normalizeRfqStatus('draft')).toBe('GENERATED');
    expect(normalizeRfqStatus('GENERATED')).toBe('GENERATED');
  });

  it('passes every visible business status through untouched', () => {
    for (const status of [
      'RECEIVED',
      'QUOTED',
      'SENT',
      'CHANGE_REQUESTED',
      'ACCEPTED',
      'REJECTED',
    ]) {
      expect(normalizeRfqStatus(status)).toBe(status);
    }
  });
});

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
