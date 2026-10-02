import { describe, expect, it } from 'vitest';

import { normalizeRfqStatus } from '@/lib/api/rfqs';

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
