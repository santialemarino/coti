import { describe, expect, it } from 'vitest';

import { customerReplyKind } from '@/app/(protected)/rfqs/_components/customer-reply';

describe('customerReplyKind', () => {
  it.each([
    ['CUSTOMER_ACCEPTS', null, 'ACCEPTS'],
    ['CUSTOMER_ACCEPTS', 'SIGNAL', 'ACCEPTS'],
    ['CUSTOMER_REJECTS', null, 'REJECTS'],
    ['CUSTOMER_QUESTION', null, 'QUESTION'],
    ['NO_REQUEST', null, 'WROTE'],
  ] as const)('reads %s (%s) as %s', (reason, replyAcceptance, kind) => {
    expect(customerReplyKind({ reason, replyAcceptance })).toBe(kind);
  });
});
