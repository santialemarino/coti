import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { getBranches } from '@/lib/api/branches';
import { apiRequest } from '@/lib/api/client';
import type { RfqChannel, RfqListItem, RfqRecord } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

function mapListItem(item: RfqListItem): RfqRecord {
  return {
    id: item.id,
    quoteNumber: item.quote_number,
    client: item.client ?? '',
    createdAt: item.created_at,
    channel: item.channel as RfqChannel,
    seller: item.seller,
    sellerId: item.seller_id,
    branch: item.branch,
    branchId: item.branch_id,
    quoteId: item.quote_id,
    itemCount: item.item_count,
    reviewCount: item.review_count,
    total: item.total ?? undefined,
    status: normalizeRfqStatus(item.status),
    needsFollowup: item.needs_followup,
    archived: item.archived_at != null,
  };
}

/*
 * Archived orders come down with the rest and the screens filter them out by default. They are a
 * handful of rows and the whole list is already filtered client-side, so fetching them is what makes
 * "archivados" a filter rather than a second round trip.
 */
async function fetchRfqs(): Promise<RfqRecord[]> {
  const items = await apiRequest<RfqListItem[]>({ path: '/v1/rfqs?include_archived=true' });
  return (items ?? []).map(mapListItem);
}

/*
 * Loads the queue once for whichever screen needs it. The home screen and the /rfqs subtree both
 * render the same list, so the fetch and the mapping live here rather than in each of their layouts.
 */
export async function RfqQueueProvider({ children }: { children: React.ReactNode }) {
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
      userId={session?.userId ?? ''}
      isAdmin={session?.role === ADMIN_ROLE}
    >
      {children}
    </RfqListProvider>
  );
}
