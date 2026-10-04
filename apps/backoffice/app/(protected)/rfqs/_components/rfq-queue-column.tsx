'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { useHeldWhileClosed, useScrollLane } from '@repo/ui/hooks';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewSwitch } from '@/app/(protected)/rfqs/_components/rfq-view-switch';
import { queueSelection } from '@/config/routes';

/*
 * The queue's column, kept in the shell's layout so it outlives navigation and can slide out as well
 * as in. The box animates its width; the rail keeps its own and rides the box's edge, never squeezed.
 */
export function RfqQueueColumn() {
  const pathname = usePathname();
  const t = useTranslations('rfqs');
  const { records, hasQueue, reload } = useRfqList();
  const lane = useScrollLane<HTMLElement>();

  const selection = queueSelection(pathname);
  const open = hasQueue && selection.inQueue;
  // The selected row stays lit while the column slides away from it.
  const activeRfqId = useHeldWhileClosed(selection.rfqId, open);

  // A layout is never re-rendered by navigation, so the list is read again whenever it comes back
  // into view: the column opening, or the window regaining focus on it.
  const wasOpen = useRef(open);
  useEffect(() => {
    if (open && !wasOpen.current) void reload();
    wasOpen.current = open;
  }, [open, reload]);

  useEffect(() => {
    if (!open) return;
    const refresh = () => void reload();
    window.addEventListener('focus', refresh);
    return () => window.removeEventListener('focus', refresh);
  }, [open, reload]);

  return (
    <div
      data-open={open}
      inert={!open}
      className="flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start sticky top-16 overflow-clip [--queue-width:clamp(240px,22vw,320px)] transition-[width] duration-200 ease-in-out-soft data-[open=true]:w-(--queue-width) data-[open=true]:ease-out-soft motion-reduce:transition-none"
    >
      {/* Held to the viewport under the 64px header and scrolled on its own, so a queue longer than
          the order never stretches the page past the detail and leaves it facing blank space. */}
      <aside
        ref={lane}
        className="flex w-(--queue-width) h-full shrink-0 flex-col bg-background border-r border-border scroll-area scroll-lane"
      >
        {/* Aligned with the rows below: the title with their text, the switch with their edge. */}
        <div className="flex items-center justify-between pt-3 pr-2 pb-1 pl-5 gap-x-2 bg-background sticky top-0 z-10">
          <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">
            {t('list.title')}
          </p>
          <RfqViewSwitch view="queue" />
        </div>
        <RfqSidebarList records={records} activeRfqId={activeRfqId} />
      </aside>
    </div>
  );
}
