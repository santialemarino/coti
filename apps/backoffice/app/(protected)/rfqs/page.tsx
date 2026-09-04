'use client';

import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

export default function RfqsPage() {
  const { records, activeBranchId } = useRfqList();

  return (
    <div className="flex flex-1 items-stretch bg-body-background">
      <main className="min-w-0 flex-1 px-6 pt-10 pb-8 lg:px-10">
        <RfqDashboard initialRecords={records} activeBranchId={activeBranchId} />
      </main>
    </div>
  );
}
