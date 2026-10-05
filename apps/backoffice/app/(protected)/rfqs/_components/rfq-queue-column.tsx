'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import { isOrdersPath, queueSelection } from '@/config/routes';

/*
 * The queue's column, kept in the shell's layout so it outlives navigation and can slide out as well
 * as in. The box animates its width; the rail keeps its own and rides the box's edge, never squeezed.
 */
export function RfqQueueColumn() {
  const pathname = usePathname();
  const t = useTranslations('rfqs');
  const { records, hasQueue, loadFailed, reload } = useRfqList();

  const selection = queueSelection(pathname);
  const open = hasQueue && !loadFailed && selection.inQueue;
  // The selected row stays lit while the column slides away from it.
  const activeRfqId = useHeldWhileClosed(selection.rfqId, open);

  // A layout is never re-rendered by navigation, so the list is read again on every move within the
  // orders — the queue, an order, the table — and when the window regains focus on them.
  const inOrders = hasQueue && isOrdersPath(pathname);
  const lastPath = useRef(pathname);
  useEffect(() => {
    if (pathname === lastPath.current) return;
    lastPath.current = pathname;
    if (inOrders) void reload();
  }, [pathname, inOrders, reload]);

  useEffect(() => {
    if (!inOrders) return;
    const refresh = () => void reload();
    window.addEventListener('focus', refresh);
    return () => window.removeEventListener('focus', refresh);
  }, [inOrders, reload]);

  return (
    <div
      data-open={open}
      inert={!open}
      className="flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start transition-[width] duration-300 ease-in-out-soft data-[open=true]:w-(--queue-width) motion-reduce:transition-none sticky top-16 overflow-clip [--queue-width:clamp(240px,22vw,320px)]"
    >
      {/* Held to the viewport under the 64px header and scrolled on its own, so a long queue never
          stretches the page. No `scroll-lane`: its margin appears once the list overflows and would
          shift the whole rail sideways, so the thin thumb sits in the list's own padding instead. */}
      <aside className="flex flex-col w-(--queue-width) h-full shrink-0 bg-background border-r border-border scroll-area">
        {/* Aligned with the rows below: the title with their text, the switch with their edge. */}
        <div className="flex items-center justify-between pt-3 pr-2 pb-1 pl-5 gap-x-2 bg-background sticky top-0 z-10">
          <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">
            {t('list.title')}
          </p>
          <RfqViewLink to="table" />
        </div>
        <RfqSidebarList records={records} activeRfqId={activeRfqId} />
      </aside>
    </div>
  );
}
