'use client';

import * as React from 'react';
import { ChevronLeftIcon, ChevronRightIcon, MoreHorizontalIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Button } from './button';

/* Pages either side of the current one before collapsing to an ellipsis. */
const SIBLINGS = 1;

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

function Pagination({ page, pageCount, onPageChange, labels, className }: PaginationProps) {
  const pages = buildPages(page, pageCount);

  return (
    <nav
      data-slot="pagination"
      aria-label={labels.page}
      className={cn('flex items-center justify-center gap-x-1', className)}
    >
      <Button
        variant="ghost"
        size="sm"
        aria-label={labels.previous}
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
      >
        <ChevronLeftIcon aria-hidden="true" />
        <span className="hidden sm:inline">{labels.previous}</span>
      </Button>

      {pages.map((value, index) =>
        value === null ? (
          <span
            // The gap's position is its identity; two collapsed runs can't share an index.
            key={`gap-${index}`}
            aria-hidden="true"
            className="grid size-8 place-items-center text-foreground-subtle"
          >
            <MoreHorizontalIcon className="size-4" />
          </span>
        ) : (
          <Button
            key={value}
            variant={value === page ? 'outline' : 'ghost'}
            size="icon-sm"
            aria-current={value === page ? 'page' : undefined}
            onClick={() => onPageChange(value)}
          >
            {value}
          </Button>
        ),
      )}

      <Button
        variant="ghost"
        size="sm"
        aria-label={labels.next}
        disabled={page >= pageCount}
        onClick={() => onPageChange(page + 1)}
      >
        <span className="hidden sm:inline">{labels.next}</span>
        <ChevronRightIcon aria-hidden="true" />
      </Button>
    </nav>
  );
}

export { Pagination };
