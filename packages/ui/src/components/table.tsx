import * as React from 'react';

import { cn } from '../lib/utils';

type TableAlign = 'start' | 'end' | 'center';

/* Copy reads from a common left edge, figures line up by place value, a one-control column centres. */
const ALIGN_CLASSES: Record<TableAlign, string> = {
  start: 'text-left',
  end: 'text-right tabular-nums',
  center: 'text-center',
};

/*
 * Column widths for `layout="fixed"`, so the same kind of column is the same width in every table.
 * Text columns take no preset: they share whatever width is left.
 */
const TABLE_COL = {
  select: 'w-12',
  index: 'w-12',
  actions: 'w-24',
  actionsWide: 'w-28',
  badge: 'w-28',
  date: 'w-32',
  quantity: 'w-32',
  status: 'w-36',
  money: 'w-36',
} as const;

/* The scroll container is part of the component so a wide table never widens the page itself. */
function Table({
  className,
  layout = 'auto',
  ...props
}: React.ComponentProps<'table'> & {
  /* `fixed` sizes columns from their headings, so content never pushes a neighbour sideways. */
  layout?: 'auto' | 'fixed';
}) {
  return (
    <div data-slot="table-container" className="relative w-full overflow-x-auto">
      <table
        data-slot="table"
        className={cn(
          'w-full caption-bottom border-collapse text-paragraph-sm',
          layout === 'fixed' && 'table-fixed',
          className,
        )}
        {...props}
      />
    </div>
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<'thead'>) {
  return (
    <thead
      data-slot="table-header"
      className={cn('bg-sunken [&_tr]:border-b [&_tr]:border-border', className)}
      {...props}
    />
  );
}

function TableBody({ className, ...props }: React.ComponentProps<'tbody'>) {
  return (
    <tbody
      data-slot="table-body"
      className={cn('[&_tr:last-child]:border-0', className)}
      {...props}
    />
  );
}

function TableFooter({ className, ...props }: React.ComponentProps<'tfoot'>) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn('bg-sunken border-t border-border text-paragraph-sm-medium', className)}
      {...props}
    />
  );
}

function TableRow({
  className,
  interactive = false,
  ...props
}: React.ComponentProps<'tr'> & {
  /* Only a row that does something on click gets a hover; on any other row it promises an action. */
  interactive?: boolean;
}) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        'border-b border-border transition-colors duration-150 ease-out-soft data-[state=selected]:bg-accent',
        interactive && 'cursor-pointer hover:bg-muted/60 active:bg-muted',
        className,
      )}
      {...props}
    />
  );
}

function TableHead({
  className,
  align = 'start',
  ...props
}: Omit<React.ComponentProps<'th'>, 'align'> & { align?: TableAlign }) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        'h-10 px-3 align-middle whitespace-nowrap text-paragraph-xs-semibold text-foreground-muted',
        ALIGN_CLASSES[align],
        className,
      )}
      {...props}
    />
  );
}

function TableCell({
  className,
  align = 'start',
  ...props
}: Omit<React.ComponentProps<'td'>, 'align'> & { align?: TableAlign }) {
  return (
    <td
      data-slot="table-cell"
      className={cn('px-3 py-2.5 align-middle text-foreground', ALIGN_CLASSES[align], className)}
      {...props}
    />
  );
}

function TableCaption({ className, ...props }: React.ComponentProps<'caption'>) {
  return (
    <caption
      data-slot="table-caption"
      className={cn('mt-4 text-paragraph-sm text-foreground-muted', className)}
      {...props}
    />
  );
}

export {
  TABLE_COL,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
};
export type { TableAlign };
