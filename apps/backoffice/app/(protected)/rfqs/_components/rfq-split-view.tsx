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
      {/* Held to the viewport under the 64px header and scrolled on its own, so a queue longer than
          the order never stretches the page past the detail and leaves it facing blank space. */}
      <aside className="w-[22%] min-w-[240px] max-w-[320px] h-[calc(100dvh-4rem)] shrink-0 self-start bg-background border-r border-border sticky top-16 scroll-area scroll-lane">
        <RfqSidebarList records={records} activeRfqId={activeRfqId} />
      </aside>
      <main className="min-w-0 flex-1 px-6 py-6 lg:px-10">{children}</main>
    </div>
  );
}
