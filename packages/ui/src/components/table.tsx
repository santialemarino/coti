import * as React from 'react';

import { cn } from '../lib/utils';

type TableAlign = 'start' | 'end' | 'center';

/* Copy reads from a common left edge, figures line up by place value, a one-control column centres. */
const ALIGN_CLASSES: Record<TableAlign, string> = {
  start: 'text-left',
  end: 'text-right',
  center: 'text-center',
};

/*
 * Every column declares what it holds, and the kind decides its width and alignment, so the same
 * kind of column looks the same in every table. `text` is the only kind without a width: the table
 * hands its slack to the text columns, so the figures keep their width and hug their edge.
 */
const TABLE_COLUMNS = {
  text: { width: '', align: 'start', figure: false },
  short: { width: 'w-32 min-w-32', align: 'start', figure: false },
  date: { width: 'w-32 min-w-32', align: 'start', figure: true },
  status: { width: 'w-36 min-w-36', align: 'start', figure: false },
  index: { width: 'w-12 min-w-12', align: 'end', figure: true },
  count: { width: 'w-28 min-w-28', align: 'end', figure: true },
  quantity: { width: 'w-32 min-w-32', align: 'end', figure: true },
  money: { width: 'w-36 min-w-36', align: 'end', figure: true },
  quantityInput: { width: 'w-44 min-w-44', align: 'end', figure: true },
  moneyInput: { width: 'w-44 min-w-44', align: 'end', figure: true },
  select: { width: 'w-12 min-w-12', align: 'center', figure: false },
  actions: { width: 'w-24 min-w-24', align: 'center', figure: false },
  actionsWide: { width: 'w-32 min-w-32', align: 'center', figure: false },
} as const satisfies Record<string, { width: string; align: TableAlign; figure: boolean }>;

type TableColumnKind = keyof typeof TABLE_COLUMNS;

function columnClasses(kind: TableColumnKind) {
  const column = TABLE_COLUMNS[kind];
  return cn(ALIGN_CLASSES[column.align], column.figure && 'whitespace-nowrap tabular-nums');
}

/*
 * Auto layout on purpose: a squeezed table shrinks its text columns to their longest word and then
 * scrolls, where a fixed layout would crush them. `min-w` on every sized column is what holds it.
 */
function Table({ className, ...props }: React.ComponentProps<'table'>) {
  return (
    <div data-slot="table-container" className="relative w-full overflow-x-auto">
      <table
        data-slot="table"
        className={cn('w-full caption-bottom border-collapse text-paragraph-sm', className)}
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
  kind,
  ...props
}: Omit<React.ComponentProps<'th'>, 'align'> & { kind: TableColumnKind }) {
  return (
    <th
      data-slot="table-head"
      data-kind={kind}
      className={cn(
        'h-10 px-3 align-middle whitespace-nowrap text-paragraph-xs-semibold text-foreground-muted',
        TABLE_COLUMNS[kind].width,
        columnClasses(kind),
        className,
      )}
      {...props}
    />
  );
}

/* A cell takes its column's kind so it aligns with its heading. */
function TableCell({
  className,
  kind = 'text',
  ...props
}: Omit<React.ComponentProps<'td'>, 'align'> & { kind?: TableColumnKind }) {
  return (
    <td
      data-slot="table-cell"
      data-kind={kind}
      className={cn('px-3 py-2.5 align-middle text-foreground', columnClasses(kind), className)}
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
  TABLE_COLUMNS,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
};
export type { TableColumnKind };
