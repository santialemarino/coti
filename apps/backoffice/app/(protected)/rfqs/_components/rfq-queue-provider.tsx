import { unstable_rethrow } from 'next/navigation';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { getBranches } from '@/lib/api/branches';
import { apiRequest } from '@/lib/api/client';
import { mapListItem, type RfqListItem, type RfqRecord } from '@/lib/api/rfqs';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

/*
 * Archived orders come down with the rest and the screens filter them out by default. They are a
 * handful of rows and the whole list is already filtered client-side, so fetching them is what makes
 * "archivados" a filter rather than a second round trip.
 */
async function fetchRfqs(): Promise<RfqRecord[]> {
  const items = await apiRequest<RfqListItem[]>({ path: '/v1/rfqs?include_archived=true' });
  return (items ?? []).map(mapListItem);
}

// Null when the read fails, which fails the queue screens only: the rest of the shell still renders.
async function readQueue(): Promise<RfqRecord[] | null> {
  try {
    return await fetchRfqs();
  } catch (error) {
    unstable_rethrow(error);
    return null;
  }
}

/*
 * Loads the queue once for the whole signed-in shell. The column that shows it outlives every
 * navigation, so the list lives above the pages; the queue screens and the table read the same one.
 */
export async function RfqQueueProvider({ children }: { children: React.ReactNode }) {
  const [session, branches] = await Promise.all([getSession(), getBranches()]);
  const isAdmin = session?.role === ADMIN_ROLE;
  // A seller with no branch has no queue; their own settings stay reachable without one.
  const hasQueue = isAdmin || branches.length > 0;
  const [records, activeBranchId] = hasQueue
    ? await Promise.all([readQueue(), getEffectiveBranchId(branches)])
    : [[], undefined];

  return (
    <RfqListProvider
      records={records ?? []}
      hasQueue={hasQueue}
      loadFailed={records === null}
      activeBranchId={activeBranchId ?? null}
      userName={session?.name ?? ''}
      userId={session?.userId ?? ''}
      isAdmin={isAdmin}
    >
      {children}
    </RfqListProvider>
  );
}
