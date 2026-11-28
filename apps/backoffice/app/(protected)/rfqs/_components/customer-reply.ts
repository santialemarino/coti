import type { CustomerReply } from '@/lib/api/rfqs';

// How the seller reads a reply that left a sent quote as it was.
export type CustomerReplyKind = 'ACCEPTS' | 'REJECTS' | 'QUESTION' | 'WROTE';

export const CUSTOMER_REPLY_TONES = {
  ACCEPTS: 'success',
  REJECTS: 'warning',
  QUESTION: 'neutral',
  WROTE: 'neutral',
} as const satisfies Record<CustomerReplyKind, string>;

export function customerReplyKind(reply: Pick<CustomerReply, 'reason' | 'replyAcceptance'>) {
  if (reply.replyAcceptance === 'SIGNAL' || reply.reason === 'CUSTOMER_ACCEPTS') return 'ACCEPTS';
  if (reply.reason === 'CUSTOMER_REJECTS') return 'REJECTS';
  if (reply.reason === 'CUSTOMER_QUESTION') return 'QUESTION';
  return 'WROTE';
}
