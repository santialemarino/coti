'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { motion } from 'motion/react';
import { useTranslations } from 'next-intl';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { cn } from '@repo/ui/lib';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import {
  SettingsNav,
  type SettingsNavItem,
} from '@/app/(protected)/settings/_components/settings-nav';
import { isSettingsPath, queueSelection } from '@/config/routes';

type ColumnSection = 'queue' | 'settings';

interface ContextColumnProps {
  // The settings sections the caller is offered; none closes the column on the settings pages.
  settingsNav: SettingsNavItem[];
}

/*
 * The shell's one left column, kept in the layout so it outlives navigation and can slide out as
 * well as in. Each section that has a list puts it here — the queue, the settings sections — and
 * the rest leave it closed. The box animates its width; the content keeps its own and rides the
 * box's edge, never squeezed.
 */
export function ContextColumn({ settingsNav }: ContextColumnProps) {
  const pathname = usePathname();
  const t = useTranslations('rfqs');
  const tSettings = useTranslations('settings');
  const { records, hasQueue, reload } = useRfqList();

  const selection = queueSelection(pathname);
  const live: ColumnSection | null =
    hasQueue && selection.inQueue
      ? 'queue'
      : settingsNav.length > 0 && isSettingsPath(pathname)
        ? 'settings'
        : null;
  const open = live !== null;
  // What the column showed stays on it while it slides away, the selected row included.
  const section = useHeldWhileClosed(live, open);
  const activeRfqId = useHeldWhileClosed(selection.rfqId, live === 'queue');

  // A layout is never re-rendered by navigation, so the list is read again whenever it comes back
  // into view: the queue showing again, or the window regaining focus on it.
  const queueShown = live === 'queue';
  const wasShown = useRef(queueShown);
  useEffect(() => {
    if (queueShown && !wasShown.current) void reload();
    wasShown.current = queueShown;
  }, [queueShown, reload]);

  useEffect(() => {
    if (!queueShown) return;
    const refresh = () => void reload();
    window.addEventListener('focus', refresh);
    return () => window.removeEventListener('focus', refresh);
  }, [queueShown, reload]);

  return (
    <div
      data-open={open}
      inert={!open}
      className="flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start sticky top-16 overflow-clip [--column-width:clamp(240px,22vw,320px)] transition-[width] duration-300 ease-in-out-soft data-[open=true]:w-(--column-width) motion-reduce:transition-none"
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
        <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">{title}</p>
        {action}
      </div>
      {children}
    </div>
  );
}
