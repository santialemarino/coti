'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { motion } from 'motion/react';
import { useTranslations } from 'next-intl';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { cn } from '@repo/ui/lib';
import { SettingsNav, type SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import { isOrdersPath, isSettingsPath, queueSelection } from '@/config/routes';

type ColumnSection = 'queue' | 'settings';

interface ContextColumnProps {
  // The settings sections the caller is offered; none closes the column on the settings pages.
  settingsNav: SettingsNavItem[];
}

/*
 * The shell's one left column: the queue or the settings sections, closed elsewhere. It lives in the
 * layout so it can slide out as well as in; its content keeps its width and rides the box's edge.
 */
export function ContextColumn({ settingsNav }: ContextColumnProps) {
  const pathname = usePathname();
  const t = useTranslations('rfqs');
  const tSettings = useTranslations('settings');
  const { records, hasQueue, loadFailed, reload } = useRfqList();

  const selection = queueSelection(pathname);
  const live: ColumnSection | null =
    hasQueue && !loadFailed && selection.inQueue
      ? 'queue'
      : settingsNav.length > 0 && isSettingsPath(pathname)
        ? 'settings'
        : null;
  const open = live !== null;
  // What the column showed stays on it while it slides away, the selected row included.
  const section = useHeldWhileClosed(live, open);
  const activeRfqId = useHeldWhileClosed(selection.rfqId, live === 'queue');

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
      // Below lg the settings sections sit above the page instead: beside them the forms would be
      // too narrow. On a phone the queue shows the list or the order, never both side by side.
      className={cn(
        'flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start sticky top-16 overflow-clip [--column-width:clamp(240px,22vw,320px)] transition-[width] duration-300 ease-in-out-soft data-[open=true]:w-(--column-width) motion-reduce:transition-none',
        section === 'settings' && 'max-lg:hidden',
        section === 'queue' &&
          (selection.rfqId ? 'max-md:hidden' : 'max-md:[--column-width:100vw]'),
      )}
    >
      {/* Held to the viewport under the 64px header and scrolled on its own, so a long queue never
          stretches the page; `layoutScroll` lets the stacks measure their moves against that scroll.
          No `scroll-lane`: its margin appears once the list overflows and would shift the whole rail
          sideways, so the thin thumb sits in the list's own padding instead. */}
      <motion.aside
        layoutScroll
        className="flex w-(--column-width) h-full shrink-0 flex-col bg-background border-r border-border scroll-area"
      >
        {/* Both stay mounted so the queue keeps its open stacks across a visit to settings; the one
            on show fades in each time it returns. */}
        {hasQueue ? (
          <ColumnPanel
            hidden={section !== 'queue'}
            title={t('list.title')}
            action={<RfqViewLink to="table" />}
          >
            <RfqSidebarList records={records} activeRfqId={activeRfqId} />
          </ColumnPanel>
        ) : null}
        {settingsNav.length > 0 ? (
          <ColumnPanel hidden={section !== 'settings'} title={tSettings('title')}>
            <SettingsNav title={tSettings('title')} items={settingsNav} />
          </ColumnPanel>
        ) : null}
      </motion.aside>
    </div>
  );
}

interface ColumnPanelProps {
  hidden: boolean;
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}

function ColumnPanel({ hidden, title, action, children }: ColumnPanelProps) {
  return (
    <div
      className={cn(
        'flex flex-col animate-in fade-in-0 duration-300 ease-in-out-soft',
        hidden && 'hidden',
      )}
    >
      {/* One height whether or not there is an action, so the title never moves between sections;
          aligned with the rows below — the title with their text, the action with their edge. */}
      <div className="flex h-14 items-center justify-between pr-3 pl-6 gap-x-2 bg-background sticky top-0 z-10">
        <h2 className="text-paragraph-xs-medium text-foreground-subtle uppercase">{title}</h2>
        {action}
      </div>
      {children}
    </div>
  );
}
