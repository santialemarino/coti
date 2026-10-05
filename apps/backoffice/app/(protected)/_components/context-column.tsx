'use client';

import { useEffect, useRef } from 'react';
import { usePathname } from 'next/navigation';
import { motion } from 'motion/react';
import { useTranslations } from 'next-intl';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { cn } from '@repo/ui/lib';
import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { SettingsNav, type SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { RfqCreateButton } from '@/app/(protected)/rfqs/_components/rfq-create-button';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import { isOrdersPath } from '@/config/routes';
import { useColumnSection } from '@/hooks/use-column-section';
import { useMediaQuery } from '@/hooks/use-media-query';
import type { Onboarding } from '@/lib/api/onboarding';

// Tailwind's `lg`, where the column stops being the page and sits beside it.
const WIDE = '(min-width: 64rem)';

interface ContextColumnProps {
  // The settings sections the caller is offered; none closes the column on the settings pages.
  settingsNav: SettingsNavItem[];
  // An administrator's unfinished setup card; below lg the queue's landing carries it.
  onboarding: Onboarding | null;
}

/*
 * The shell's one left column: the queue or the settings sections, closed elsewhere. It lives in the
 * layout so it can slide out as well as in; its content keeps its width and rides the box's edge.
 */
export function ContextColumn({ settingsNav, onboarding }: ContextColumnProps) {
  const pathname = usePathname();
  const t = useTranslations('rfqs');
  const tSettings = useTranslations('settings');
  const { records, hasQueue, reload } = useRfqList();
  // Server renders assume a wide screen, the shape the column has there.
  const wide = useMediaQuery(WIDE, true);

  const { section: live, rfqId, atRoot } = useColumnSection(settingsNav);
  const open = live !== null;
  // What the column showed stays on it while it slides away, the selected row included.
  const section = useHeldWhileClosed(live, open);
  const activeRfqId = useHeldWhileClosed(rfqId, live === 'queue');
  // Below lg on a landing the column is the page's content, so it is its main landmark and title.
  const isPage = atRoot && !wide;

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
      // screen beside it steps aside — and what it opens takes the page, with its own way back.
      className={cn(
        'peer flex w-0 h-[calc(100dvh-4rem)] shrink-0 justify-end self-start sticky top-16 overflow-clip [--column-width:clamp(240px,22vw,320px)] transition-[width] duration-300 ease-in-out-soft data-[open=true]:w-(--column-width) motion-reduce:transition-none',
        atRoot
          ? 'max-lg:static max-lg:h-auto max-lg:flex-1 max-lg:self-stretch max-lg:[--column-width:100%] max-lg:transition-none max-lg:[view-transition-name:context-list]'
          : 'max-lg:hidden',
      )}
    >
      {/* From lg up it is held under the header and scrolls on its own (`layoutScroll` measures the
          stacks against that); no `scroll-lane`, whose margin would shift the rail once it overflows. */}
      <motion.aside
        layoutScroll
        role={isPage ? 'main' : undefined}
        className="flex w-(--column-width) h-full shrink-0 flex-col bg-background border-r border-border scroll-area max-lg:h-auto max-lg:overflow-visible max-lg:bg-transparent max-lg:border-r-0"
      >
        {/* Both stay mounted so the queue keeps its open stacks across a visit to settings; the one
            on show fades in each time it returns. */}
        {hasQueue ? (
          <ColumnPanel
            hidden={section !== 'queue'}
            title={t('list.title')}
            isPage={isPage}
            action={
              <>
                {/* Below lg the list is the landing, so it carries the screen's first action. */}
                <RfqCreateButton className="lg:hidden" />
                {/* Button has no responsive size: below lg this is its default size, by hand. */}
                <RfqViewLink to="table" className="max-lg:h-9 max-lg:px-4 max-lg:gap-x-2" />
              </>
            }
          >
            {onboarding ? (
              <OnboardingChecklist
                onboarding={onboarding}
                placement="home"
                className="mx-6 mb-6 lg:hidden"
              />
            ) : null}
            <RfqSidebarList records={records} activeRfqId={activeRfqId} />
          </ColumnPanel>
        ) : null}
        {settingsNav.length > 0 ? (
          <ColumnPanel hidden={section !== 'settings'} title={tSettings('title')} isPage={isPage}>
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
  isPage: boolean;
  action?: React.ReactNode;
  children: React.ReactNode;
}

function ColumnPanel({ hidden, title, isPage, action, children }: ColumnPanelProps) {
  const Heading = isPage ? 'h1' : 'h2';

  return (
    <div
      className={cn(
        'flex flex-col animate-in fade-in-0 duration-300 ease-in-out-soft',
        hidden && 'hidden',
      )}
    >
      {/* From lg up one height with or without an action, the title on the rows' text and the action
          on their edge; below lg it is the page's header, laid out as PageHeader is. */}
      <div className="flex h-14 items-center justify-between pr-3 pl-6 gap-x-2 bg-background sticky top-0 z-10 max-lg:static max-lg:h-auto max-lg:flex-col max-lg:items-start max-lg:px-6 max-lg:pt-10 max-lg:pb-8 max-lg:gap-3 max-lg:bg-transparent sm:max-lg:flex-row sm:max-lg:items-end">
        <Heading className="text-paragraph-xs-medium text-foreground-subtle uppercase max-lg:text-heading-2 max-lg:text-foreground max-lg:normal-case">
          {title}
        </Heading>
        {action ? <div className="flex items-center gap-x-2 max-lg:gap-x-3">{action}</div> : null}
      </div>
      {children}
    </div>
  );
}
