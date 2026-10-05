'use client';

import { useState } from 'react';
import { usePathname } from 'next/navigation';
import { MenuIcon } from 'lucide-react';
import { motion } from 'motion/react';
import { useTranslations } from 'next-intl';

import { Button, Sheet, SheetContent, SheetTitle, SheetTrigger } from '@repo/ui/components';
import { ContextPanels, useColumnSection } from '@/app/(protected)/_components/context-panels';
import { PrimaryNav } from '@/app/(protected)/_components/primary-nav';
import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { Brand } from '@/components/brand';

interface ContextSheetProps {
  settingsNav: SettingsNavItem[];
  className?: string;
}

/*
 * Below lg the left column has no room beside the page, so the page stays on screen and this opens
 * the column full screen instead: the main navigation on top, the section's column under it.
 * Choosing anything in it closes it again.
 */
export function ContextSheet({ settingsNav, className }: ContextSheetProps) {
  const pathname = usePathname();
  const t = useTranslations('common.nav');
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState(pathname);
  const { section, rfqId } = useColumnSection(settingsNav);

  // Any navigation closes it — a link inside, the back button — adjusted during render so the next
  // screen never paints underneath it first.
  if (path !== pathname) {
    setPath(pathname);
    setOpen(false);
  }

  // Opening the screen already showing changes no path, so a press on anything that navigates closes
  // it too; an unfolding stack does not.
  function closeOnNavigate(event: React.MouseEvent) {
    if (event.target instanceof Element && event.target.closest('a, [data-navigates]')) {
      setOpen(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t('openMenu')} className={className}>
          <MenuIcon aria-hidden="true" />
        </Button>
      </SheetTrigger>
      <SheetContent side="left" aria-describedby={undefined} className="w-full sm:max-w-none">
        {/* The sheet's own padding is undone so the column's insets match the desktop column's;
            `layoutScroll` lets the stacks measure their moves against this scroll. */}
        <motion.div
          layoutScroll
          onClick={closeOnNavigate}
          className="flex flex-col flex-1 min-h-0 -m-6 pb-3 scroll-area"
        >
          <div className="flex h-14 shrink-0 items-center pl-6">
            <Brand variant="wordmark" size="md" />
          </div>
          <SheetTitle className="sr-only">{t('menu')}</SheetTitle>
          <PrimaryNav layout="stack" />
          {section ? (
            <div className="flex flex-col mt-3 border-t border-border">
              <ContextPanels section={section} activeRfqId={rfqId} settingsNav={settingsNav} />
            </div>
          ) : null}
        </motion.div>
      </SheetContent>
    </Sheet>
  );
}
