import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

/*
 * A stack of the queue column. An accepted order splits in two: «Por cerrar» while the sale still
 * waits to be paid or invoiced, «Completados» once it is closed.
 */
export type QueueStack = Exclude<RfqStatus, 'ACCEPTED'> | 'TO_CLOSE' | 'COMPLETED';

/*
 * By who has to act: what waits on the seller first, the accepted sales still open next, what waits
 * on the client or on the pipeline after, and the closed outcomes last. Decoupled from the table's
 * status order, which follows the workflow.
 */
export const QUEUE_STACK_ORDER: readonly QueueStack[] = [
  'CHANGE_REQUESTED',
  'FAILED',
  'GENERATED',
  'QUOTED',
  'TO_CLOSE',
  'SENT',
  'RECEIVED',
  'COMPLETED',
  'REJECTED',
];

// The closed outcomes: their stacks start folded away, heading only, at the foot of the column.
export const FOLDED_STACKS: ReadonlySet<QueueStack> = new Set(['COMPLETED', 'REJECTED']);

export interface QueueStatusGroup {
  stack: QueueStack;
  records: RfqRecord[];
}

export function queueStackOf(record: Pick<RfqRecord, 'status' | 'saleComplete'>): QueueStack {
  if (record.status !== 'ACCEPTED') return record.status;
  return record.saleComplete ? 'COMPLETED' : 'TO_CLOSE';
}

/*
 * The open orders by stack, in the order above. Each group keeps the list's own order — follow-ups
 * first, then newest — so the top of a stack is what to look at next.
 */
export function groupQueue(records: readonly RfqRecord[]): QueueStatusGroup[] {
  const byStack = new Map<QueueStack, RfqRecord[]>();
  for (const record of records) {
    if (record.archived) continue;
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
