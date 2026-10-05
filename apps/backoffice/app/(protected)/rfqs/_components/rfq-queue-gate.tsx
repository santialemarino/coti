'use client';

import { NoBranchScreen } from '@/app/(protected)/_components/no-branch-screen';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

// The queue screens, or why there are none for a seller nobody assigned to a branch.
export function RfqQueueGate({ children }: { children: React.ReactNode }) {
  const { hasQueue, loadFailed } = useRfqList();
  // Handed to the error boundary, the way the queue screens failed when each read its own list.
  if (loadFailed) throw new Error('The order queue could not be read.');
  return hasQueue ? children : <NoBranchScreen />;
}
