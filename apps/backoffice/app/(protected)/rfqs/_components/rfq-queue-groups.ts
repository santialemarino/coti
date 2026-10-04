import { STATUS_ORDER } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

export interface RfqQueueGroup {
  status: RfqStatus;
  records: RfqRecord[];
}

/*
 * The open orders by status, in the order the workflow moves through them. Each group keeps the
 * list's own order — follow-ups first, then newest — so the top of a stack is what to look at next.
 */
export function groupQueue(records: readonly RfqRecord[]): RfqQueueGroup[] {
  const byStatus = new Map<RfqStatus, RfqRecord[]>();
  for (const record of records) {
    if (record.archived) continue;
    const group = byStatus.get(record.status);
    if (group) group.push(record);
    else byStatus.set(record.status, [record]);
  }
  return STATUS_ORDER.flatMap((status) => {
    const group = byStatus.get(status);
    return group ? [{ status, records: group }] : [];
  });
}
