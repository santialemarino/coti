'use client';

import * as React from 'react';
import { ArrowDownIcon, ArrowUpIcon, ChevronsUpDownIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { TableHead } from './table';

export type SortOrder = 'asc' | 'desc';

/*
 * All three glyphs share one grid cell and crossfade, so the header never reflows as the sort
 * changes — a column shifting by two pixels on every click is the kind of thing that makes a table
 * feel cheap.
 */
function SortIcon({ active, order }: { active: boolean; order: SortOrder }) {
  const iconClass =
    'col-start-1 row-start-1 size-3.5 transition-[opacity,scale] duration-200 ease-out-soft';

  return (
    <span aria-hidden="true" className="grid shrink-0 group-focus-visible/sort:animate-focus-bump">
      <ChevronsUpDownIcon
        className={cn(
          iconClass,
          'text-foreground-subtle',
          active ? 'scale-0 opacity-0' : 'scale-100 opacity-100',
        )}
      />
      <ArrowUpIcon
        className={cn(
          iconClass,
          'text-primary',
          active && order === 'asc' ? 'scale-100 opacity-100' : 'scale-0 opacity-0',
        )}
      />
      <ArrowDownIcon
        className={cn(
          iconClass,
          'text-primary',
          active && order === 'desc' ? 'scale-100 opacity-100' : 'scale-0 opacity-0',
        )}
      />
    </span>
  );
}

interface SortableTableHeadProps<TColumn extends string> {
  label: string;
  column: TColumn;
  sortBy: TColumn | null;
  sortOrder: SortOrder;
  onSort: (column: TColumn) => void;
  className?: string;
}

/*
 * `aria-sort` on the cell is what a screen reader announces, so the sort state is not conveyed by
 * the glyph alone. The button drops its own outline because the icon carries the focus feedback —
 * paired with the hover colour shift, which is also the reduced-motion fallback.
 */
function SortableTableHead<TColumn extends string>({
  label,
  column,
  sortBy,
  sortOrder,
  onSort,
  className,
}: SortableTableHeadProps<TColumn>) {
  const active = sortBy === column;

  return (
    <TableHead
      aria-sort={active ? (sortOrder === 'asc' ? 'ascending' : 'descending') : 'none'}
      className={className}
    >
      <button
        type="button"
        onClick={() => onSort(column)}
        className={cn(
          'group/sort flex items-center gap-x-1.5 rounded-sm outline-none',
          'transition-colors duration-150 ease-out-soft',
          active ? 'text-foreground' : 'hover:text-foreground focus-visible:text-foreground',
        )}
      >
        {label}
        <SortIcon active={active} order={sortOrder} />
      </button>
    </TableHead>
  );
}

export { SortableTableHead, SortIcon };
