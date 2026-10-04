'use client';

import { NoBranchScreen } from '@/app/(protected)/_components/no-branch-screen';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

// The queue screens, or why there are none for a seller nobody assigned to a branch.
export function RfqQueueGate({ children }: { children: React.ReactNode }) {
  const { hasQueue } = useRfqList();
  return hasQueue ? children : <NoBranchScreen />;
}
