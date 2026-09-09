import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { getBranches } from '@/lib/api/branches';
import { apiRequest } from '@/lib/api/client';
import type { RfqChannel, RfqListItem, RfqRecord } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';

function mapListItem(item: RfqListItem): RfqRecord {
  return {
    id: item.id,
    client: item.client ?? '',
    createdAt: item.created_at,
    channel: item.channel as RfqChannel,
    seller: item.seller,
    sellerId: item.seller_id,
    branch: item.branch,
    branchId: item.branch_id,
    itemCount: item.item_count,
    total: item.total ?? undefined,
    priority: 'normal',
    status: normalizeRfqStatus(item.status),
    needsFollowup: item.needs_followup,
    archived: item.archived_at != null,
  };
}

async function fetchRfqs(): Promise<RfqRecord[]> {
  const items = await apiRequest<RfqListItem[]>({ path: '/v1/rfqs' });
  return (items ?? []).map(mapListItem);
}

export default async function RfqsLayout({ children }: { children: React.ReactNode }) {
  const [records, session, branches] = await Promise.all([
    fetchRfqs(),
    getSession(),
    getBranches(),
  ]);
  const activeBranchId = await getEffectiveBranchId(branches);

  return (
    <RfqListProvider
      records={records}
      activeBranchId={activeBranchId ?? null}
      userName={session?.name ?? ''}
    >
      {children}
    </RfqListProvider>
  );
}
