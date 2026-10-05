'use client';

import { PageShell } from '@/app/(protected)/_components/page-shell';
import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

export default function RfqsPage() {
  const { records, activeBranchId } = useRfqList();

  return (
    <PageShell>
      <RfqDashboard initialRecords={records} activeBranchId={activeBranchId} />
    </PageShell>
  );
}
