'use client';

import * as React from 'react';
import { ChevronLeftIcon, ChevronRightIcon, MoreHorizontalIcon } from 'lucide-react';
import { AnimatePresence, LayoutGroup, motion, useReducedMotion } from 'motion/react';

import { EASE, MOTION } from '../lib/motion';
import { cn } from '../lib/utils';
import { Button } from './button';

/* Pages either side of the current one before collapsing to an ellipsis. */
const SIBLINGS = 1;

const MotionButton = motion.create(Button);

interface PaginationProps {
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  labels: { previous: string; next: string; page: string };
  className?: string;
}

/*
 * Returns the page numbers to render, with `null` marking a collapsed run. Always yields the first
 * and last page plus a window around the current one, so the control's width stays stable however
 * many pages there are.
 */
function buildPages(page: number, pageCount: number): (number | null)[] {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1);

  const window = new Set<number>([1, pageCount, page]);
  for (let offset = 1; offset <= SIBLINGS; offset += 1) {
    window.add(Math.max(1, page - offset));
    window.add(Math.min(pageCount, page + offset));
  }

  const sorted = [...window].sort((a, b) => a - b);
  return sorted.flatMap((value, index) => {
    const previous = sorted[index - 1];
    return previous !== undefined && value - previous > 1 ? [null, value] : [value];
  });
}

/*
 * The window slides as the page changes, so numbers enter and leave and everything left over shifts
 * along. Both are layout animations: `popLayout` takes a leaving number out of flow immediately, so
 * its neighbours travel to their new positions instead of waiting for it to finish fading, and
 * `layout="position"` moves a box without rescaling the digit inside it. The whole row is one
 * LayoutGroup so the previous/next buttons ride the same measurement pass as the numbers between
 * them.
 */
function Pagination({ page, pageCount, onPageChange, labels, className }: PaginationProps) {
  const pages = buildPages(page, pageCount);
  const reduced = useReducedMotion();

  /*
   * Reduced motion keeps the same tree and zeroes the timings rather than rendering plain elements:
   * motion inlines its own styles during SSR, and swapped elements would strand them.
   */
  const transition = { duration: reduced ? 0 : MOTION.default, ease: EASE.outSoft };
  const layout = reduced ? (false as const) : ('position' as const);
  const appear = {
    initial: { opacity: 0, scale: 0.6 },
    animate: { opacity: 1, scale: 1 },
    exit: { opacity: 0, scale: 0.6 },
  };

  return (
    <nav
      data-slot="pagination"
      aria-label={labels.page}
      className={cn('flex items-center justify-center gap-x-1', className)}
    >
      <LayoutGroup>
        <MotionButton
          layout={layout}
          transition={transition}
          variant="ghost"
          size="sm"
          aria-label={labels.previous}
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeftIcon aria-hidden="true" />
          <span className="hidden sm:inline">{labels.previous}</span>
        </MotionButton>

        <AnimatePresence initial={false} mode="popLayout">
          {pages.map((value, index) =>
            value === null ? (
              <motion.span
                // The gap's position is its identity; two collapsed runs can't share an index.
                key={`gap-${index}`}
                layout={layout}
                transition={transition}
                {...appear}
                aria-hidden="true"
                className="grid size-8 place-items-center text-foreground-subtle"
              >
                <MoreHorizontalIcon className="size-4" />
              </motion.span>
            ) : (
              <MotionButton
                key={value}
                layout={layout}
                transition={transition}
                {...appear}
                variant={value === page ? 'outline' : 'ghost'}
                size="icon-sm"
                aria-current={value === page ? 'page' : undefined}
                onClick={() => onPageChange(value)}
              >
                {value}
              </MotionButton>
            ),
          )}
        </AnimatePresence>

        <MotionButton
          layout={layout}
          transition={transition}
          variant="ghost"
          size="sm"
          aria-label={labels.next}
          disabled={page >= pageCount}
          onClick={() => onPageChange(page + 1)}
        >
          <span className="hidden sm:inline">{labels.next}</span>
          <ChevronRightIcon aria-hidden="true" />
        </MotionButton>
      </LayoutGroup>
    </nav>
  );
}

export { Pagination };
