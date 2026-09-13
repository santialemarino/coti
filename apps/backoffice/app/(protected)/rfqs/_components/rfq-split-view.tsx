'use client';

import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';

interface RfqSplitViewProps {
  /* The order the right pane is showing, or null on the queue's own landing screen. */
  activeRfqId: string | null;
  children: React.ReactNode;
}

/*
 * The queue shell: the list of orders on the left, whatever is being worked on the right. Both the
 * home screen and an order's detail render through it, so the sidebar cannot drift between them and
 * moving from one order to the next never re-mounts the list.
 */
export function RfqSplitView({ activeRfqId, children }: RfqSplitViewProps) {
  const { records } = useRfqList();

  return (
    <div className="flex flex-1 items-stretch bg-body-background">
      <aside className="w-[22%] min-w-[240px] max-w-[320px] shrink-0 overflow-y-auto border-r border-border bg-background">
        <RfqSidebarList records={records} activeRfqId={activeRfqId} />
      </aside>
      <main className="min-w-0 flex-1 overflow-y-auto px-6 py-6 lg:px-8">{children}</main>
    </div>
  );
}
