import { unstable_rethrow } from 'next/navigation';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { getAccount, type Account } from '@/lib/api/account';
import { getBranches } from '@/lib/api/branches';
import { apiRequest } from '@/lib/api/client';
import { mapListItem, type RfqListItem, type RfqRecord } from '@/lib/api/rfqs';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

// The open queue only: the archive comes down when the table's filter asks for it.
async function fetchRfqs(): Promise<RfqRecord[]> {
  const items = await apiRequest<RfqListItem[]>({ path: '/v1/rfqs' });
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

// Null when the account cannot be read: the list keeps the switch it last knew, and the next
// refresh of the shell reads it again.
async function readAccount(): Promise<Account | null> {
  try {
    return await getAccount();
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
  const [records, activeBranchId, account] = hasQueue
    ? await Promise.all([readQueue(), getEffectiveBranchId(branches), readAccount()])
    : [[], undefined, null];

  return (
    <RfqListProvider
      records={records ?? []}
      hasQueue={hasQueue}
      loadFailed={records === null}
      activeBranchId={activeBranchId ?? null}
      userName={session?.name ?? ''}
      userId={session?.userId ?? ''}
      isAdmin={isAdmin}
      invoicingEnabled={account ? account.invoicingEnabled : null}
      invoiceOnPayment={account ? account.invoiceOnPayment : null}
    >
      {children}
    </RfqListProvider>
  );
}
