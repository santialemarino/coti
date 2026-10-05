'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { motion } from 'motion/react';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { cn } from '@repo/ui/lib';
import { ContextPanels, useColumnSection } from '@/app/(protected)/_components/context-panels';
import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { isOrdersPath } from '@/config/routes';

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
  const { hasQueue, reload } = useRfqList();

  const { section: live, rfqId, atRoot } = useColumnSection(settingsNav);
  const open = live !== null;
  // What the column showed stays on it while it slides away, the selected row included.
  const section = useHeldWhileClosed(live, open);
  const activeRfqId = useHeldWhileClosed(rfqId, live === 'queue');

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
      data-root={atRoot}
      inert={!open}
      // Below lg there is no room for both: the section's landing is the column as the page — the
      // list, which the screen beside it steps aside for — and anything opened from it takes the
      // page instead, with its own way back.
      className={cn(
        'peer flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start sticky top-16 overflow-clip [--column-width:clamp(240px,22vw,320px)] transition-[width] duration-300 ease-in-out-soft data-[open=true]:w-(--column-width) motion-reduce:transition-none',
        atRoot
          ? 'max-lg:static max-lg:h-auto max-lg:flex-1 max-lg:self-stretch max-lg:[--column-width:100%] max-lg:transition-none max-lg:[view-transition-name:context-list]'
          : 'max-lg:hidden',
      )}
    >
      {/* Held to the viewport under the 64px header and scrolled on its own, so a long queue never
          stretches the page; `layoutScroll` lets the stacks measure their moves against that scroll.
          No `scroll-lane`: its margin appears once the list overflows and would shift the whole rail
          sideways, so the thin thumb sits in the list's own padding instead. */}
      <motion.aside
        layoutScroll
        className="flex w-(--column-width) h-full shrink-0 flex-col bg-background border-r border-border scroll-area max-lg:h-auto max-lg:overflow-visible max-lg:bg-transparent max-lg:border-r-0"
      >
        <ContextPanels section={section} activeRfqId={activeRfqId} settingsNav={settingsNav} />
      </motion.aside>
    </div>
  );
}
