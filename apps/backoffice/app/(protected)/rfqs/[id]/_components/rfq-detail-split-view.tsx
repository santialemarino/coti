'use client';

import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqDetailView } from '@/app/(protected)/rfqs/[id]/_components/rfq-detail-view';
import type { RfqDetailResponse } from '@/lib/api/rfqs';

interface RfqDetailSplitViewProps {
  detail: RfqDetailResponse;
}

export function RfqDetailSplitView({ detail }: RfqDetailSplitViewProps) {
  const { records } = useRfqList();
  const activeId = detail.rfq.id;

  return (
    <div className="flex flex-1 items-stretch bg-body-background">
      {/* Sidebar — compact pedido selector */}
      <aside className="w-[22%] min-w-[240px] max-w-[320px] shrink-0 overflow-y-auto border-r border-border bg-background">
        <RfqSidebarList records={records} activeRfqId={activeId} />
      </aside>

      {/* Detail — takes all remaining space */}
      <main className="min-w-0 flex-1 overflow-y-auto px-6 py-6 lg:px-8">
        <RfqDetailView detail={detail} />
      </main>
    </div>
  );
}
