import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

/*
 * A stack of the queue column. An accepted order splits in three: «Aceptada por WhatsApp ·
 * confirmar» while an acceptance read from the customer's reply waits for the seller, «Por cerrar»
 * while the sale still waits to be paid or invoiced, «Completados» once it is closed. A sent order
 * whose customer wrote without changing it leaves «Enviado» for «El cliente escribió».
 */
export type QueueStack =
  | Exclude<RfqStatus, 'ACCEPTED'>
  | 'ACCEPTANCE_TO_CONFIRM'
  | 'CUSTOMER_WROTE'
  | 'TO_CLOSE'
  | 'COMPLETED'
  | 'ARCHIVED';

/*
 * By who has to act: an acceptance to confirm first, since the sale cannot close until it is, then
 * what else waits on the seller, the replies to read, the accepted sales still open, what waits on
 * the client or on the pipeline after, and the closed outcomes last. Decoupled from the table's
 * status order, which follows the workflow.
 */
export const QUEUE_STACK_ORDER: readonly QueueStack[] = [
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
  'ARCHIVED',
];

// The closed outcomes: their stacks start folded away, heading only, at the foot of the column.
export const FOLDED_STACKS: ReadonlySet<QueueStack> = new Set([
  'COMPLETED',
  'REJECTED',
  'ARCHIVED',
]);

export interface QueueStatusGroup {
  stack: QueueStack;
  records: RfqRecord[];
}

export function queueStackOf(
  record: Pick<
    RfqRecord,
    'status' | 'saleComplete' | 'archived' | 'acceptanceUnconfirmed' | 'customerReply'
  >,
): QueueStack {
  if (record.archived) return 'ARCHIVED';
  if (record.status === 'SENT' && record.customerReply) return 'CUSTOMER_WROTE';
  if (record.status !== 'ACCEPTED') return record.status;
  if (record.acceptanceUnconfirmed) return 'ACCEPTANCE_TO_CONFIRM';
  return record.saleComplete ? 'COMPLETED' : 'TO_CLOSE';
}

/*
 * The open orders by stack, in the order above. Each group keeps the list's own order — follow-ups
 * first, then newest — so the top of a stack is what to look at next.
 */
export function groupQueue(records: readonly RfqRecord[]): QueueStatusGroup[] {
  const byStack = new Map<QueueStack, RfqRecord[]>();
  for (const record of records) {
    const stack = queueStackOf(record);
    const group = byStack.get(stack);
    if (group) group.push(record);
    else byStack.set(stack, [record]);
  }
  return QUEUE_STACK_ORDER.flatMap((stack) => {
    const group = byStack.get(stack);
    return group ? [{ stack, records: group }] : [];
  });
}
