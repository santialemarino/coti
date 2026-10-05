'use client';

import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { motion } from 'motion/react';
import { useTranslations } from 'next-intl';

import {
  DropdownChevron,
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@repo/ui/components';
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { ContextPanels, useColumnSection } from '@/app/(protected)/_components/context-panels';
import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

interface SectionBarProps {
  settingsNav: SettingsNavItem[];
}

/*
 * Below lg the left column has no room beside the page, so the page keeps the screen and this bar,
 * present only in a section that has a column, names it and drops it open beneath itself. Choosing
 * anything in it closes it again.
 */
export function SectionBar({ settingsNav }: SectionBarProps) {
  const pathname = usePathname();
  const t = useTranslations('common.actions');
  const tRfqs = useTranslations('rfqs');
  const tSettings = useTranslations('settings');
  const { records } = useRfqList();
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState(pathname);
  const { section: live, rfqId } = useColumnSection(settingsNav);
  // The panel keeps what it showed while it leaves, even when the path that closed it has no column.
  const section = useHeldWhileClosed(live, open);
  const activeRfqId = useHeldWhileClosed(rfqId, open);

  // Any navigation closes it — a choice inside, the back button — adjusted during render so the next
  // screen never paints underneath it first.
  if (path !== pathname) {
    setPath(pathname);
    setOpen(false);
  }

  const title = live === 'queue' ? tRfqs('list.title') : tSettings('title');
  const detail =
    live === 'queue'
      ? records.filter((rfq) => !rfq.archived).length
      : settingsNav.find((item) => item.href === pathname)?.label;

  // Opening the screen already showing changes no path, so a press on anything that navigates closes
  // it too; an unfolding stack does not.
  function closeOnNavigate(event: React.MouseEvent) {
    if (event.target instanceof Element && event.target.closest('a, [data-navigates]')) {
      setOpen(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      {live ? (
        <div className="sticky top-16 z-30 flex h-12 shrink-0 items-center px-2 bg-background/85 border-b border-border backdrop-blur lg:hidden">
          <SheetTrigger asChild>
            <button
              type="button"
              className="flex w-full h-10 items-center justify-between px-3 gap-x-2 rounded-lg outline-none transition-[background-color,box-shadow] duration-150 ease-out-soft hover:bg-surface-hover active:bg-surface-active aria-expanded:bg-surface-hover focus-visible:ring-3 focus-visible:ring-ring/45"
            >
              <span className="flex min-w-0 items-baseline gap-x-2">
                <span className="shrink-0 text-paragraph-sm-medium text-foreground">{title}</span>
                <span className="truncate text-paragraph-sm text-foreground-subtle tabular-nums">
                  {detail}
                </span>
              </span>
              <DropdownChevron open={open} className="text-foreground" />
            </button>
          </SheetTrigger>
        </div>
      ) : null}
      {/* Anchored to the bar, it fades and slides a step down from under it rather than travelling
          the screen; the header and the bar stay undimmed, and a press on either closes it. */}
      <SheetContent
        side="top"
        showCloseButton={false}
        aria-describedby={undefined}
        overlayClassName="top-28"
        className="top-28 bottom-0 max-h-none border-b-0 shadow-none data-[state=open]:ease-in-out-soft data-[state=closed]:ease-in-out-soft data-[state=closed]:duration-300 data-[state=open]:slide-in-from-top-2 data-[state=closed]:slide-out-to-top-2 data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0 lg:hidden"
      >
        {/* The sheet's own padding is undone so the column's insets match the desktop column's;
            `layoutScroll` lets the stacks measure their moves against this scroll. */}
        <motion.div
          layoutScroll
          onClick={closeOnNavigate}
          className="flex flex-col flex-1 min-h-0 -m-6 pb-3 scroll-area"
        >
          <SheetTitle className="sr-only">{title}</SheetTitle>
          <ContextPanels section={section} activeRfqId={activeRfqId} settingsNav={settingsNav} />
          <SheetClose className="sr-only">{t('close')}</SheetClose>
        </motion.div>
      </SheetContent>
    </Sheet>
  );
}
