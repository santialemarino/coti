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
  paid_at: '2026-09-28T15:00:00.000Z',
  paid_by: 'Ana',
  invoice_waived_at: '2026-09-28T16:00:00.000Z',
  invoice_waived_by: 'Ana',
  invoiced: false,
  invoice_pending: true,
  sale_complete: true,
  acceptance_unconfirmed: false,
  customer_reply: {
    reason: 'CUSTOMER_ACCEPTS',
    reply_acceptance: 'SIGNAL',
    created_at: '2026-09-29T10:00:00.000Z',
  },
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
      paidAt: '2026-09-28T15:00:00.000Z',
      invoiceWaivedAt: '2026-09-28T16:00:00.000Z',
      invoiced: false,
      invoicePending: true,
      saleComplete: true,
      acceptanceUnconfirmed: false,
      customerReply: {
        reason: 'CUSTOMER_ACCEPTS',
        replyAcceptance: 'SIGNAL',
        createdAt: '2026-09-29T10:00:00.000Z',
      },
    });
  });

  it('reads an unconfirmed acceptance and no reply', () => {
    const mapped = mapListItem({
      ...RAW,
      status: 'ACCEPTED',
      acceptance_unconfirmed: true,
      customer_reply: null,
    });

    expect(mapped).toMatchObject({ acceptanceUnconfirmed: true, customerReply: null });
  });

  // A row from an API that predates the fields reads as nothing to confirm and nothing to read.
  it('defaults the reply fields when the row carries none', () => {
    const older: RfqListItem = { ...RAW };
    delete older.acceptance_unconfirmed;
    delete older.customer_reply;

    expect(mapListItem(older)).toMatchObject({ acceptanceUnconfirmed: false, customerReply: null });
  });
});
