'use client';

import { useTranslations } from 'next-intl';

import {
  Callout,
  Card,
  CardHeader,
  CardTitle,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import type { ChangeRequestDiff, DiffDiscountLine, DiffLineItem } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

interface RfqChangeDiffProps {
  diff: ChangeRequestDiff;
}

interface AlignedRow {
  original: DiffLineItem | null;
  requested: DiffLineItem | null;
  changeType: 'modified' | 'added' | 'removed' | null;
}

function buildAlignedRows(original: DiffLineItem[], requested: DiffLineItem[]): AlignedRow[] {
  const rows: AlignedRow[] = [];
  let origIdx = 0;

  for (const reqItem of requested) {
    if (reqItem.change_type === 'added') {
      rows.push({ original: null, requested: reqItem, changeType: 'added' });
    } else if (reqItem.change_type === 'removed') {
      rows.push({ original: original[origIdx] ?? null, requested: null, changeType: 'removed' });
      origIdx++;
    } else {
      rows.push({
        original: original[origIdx] ?? null,
        requested: reqItem,
        changeType: reqItem.changed ? (reqItem.change_type ?? 'modified') : null,
      });
      origIdx++;
    }
  }

  while (origIdx < original.length) {
    rows.push({ original: original[origIdx] ?? null, requested: null, changeType: null });
    origIdx++;
  }

  return rows;
}

function DiffItemCell({
  item,
  changeType,
  fmt,
}: {
  item: DiffLineItem | null;
  changeType: 'modified' | 'added' | 'removed' | null;
  fmt: ReturnType<typeof useFormatters>;
}) {
  const t = useTranslations('rfqs');

  // A line the other side does not have: the row is held so both panels stay in step.
  if (!item) {
    return (
      <TableRow className="bg-muted/30 hover:bg-muted/30">
        <TableCell className="text-foreground-subtle">—</TableCell>
        <TableCell className="text-right text-foreground-subtle">—</TableCell>
        <TableCell className="text-right text-foreground-subtle">—</TableCell>
      </TableRow>
    );
  }

  const isRemoved = changeType === 'removed';
  const isAdded = changeType === 'added';
  const isModified = changeType === 'modified';

  const rowTone = isRemoved
    ? 'bg-danger-subtle hover:bg-danger-subtle'
    : isAdded
      ? 'bg-success-subtle hover:bg-success-subtle'
      : isModified
        ? 'bg-warning-subtle hover:bg-warning-subtle'
        : undefined;

  /* `-foreground`, not `-base`: the base steps are tuned for fills and carry no text contrast. */
  const changeLabel = changeType ? (
    <span
      className={cn(
        'ml-2 text-paragraph-xs-medium',
        isRemoved
          ? 'text-danger-foreground'
          : isAdded
            ? 'text-success-foreground'
            : 'text-warning-foreground',
      )}
    >
      {t(`detail.diff.changeTypes.${changeType}`)}
    </span>
  ) : null;

  return (
    <TableRow className={rowTone}>
      <TableCell className="text-foreground">
        <span
          className={cn(
            isRemoved && 'line-through text-foreground-muted',
            isAdded && 'text-paragraph-sm-medium text-foreground',
          )}
        >
          {item.description}
        </span>
        {changeLabel}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {isRemoved ? (
          <span className="text-foreground-muted">—</span>
        ) : (
          <span>
            {fmt.value(Number(item.quantity))} {item.unit ?? ''}
          </span>
        )}
      </TableCell>
      <TableCell className="text-right text-paragraph-sm-medium tabular-nums">
        {isRemoved ? (
          <span className="text-foreground-muted">—</span>
        ) : item.unit_price != null ? (
          fmt.currency(item.unit_price)
        ) : (
          <span className="text-foreground-subtle">—</span>
        )}
      </TableCell>
    </TableRow>
  );
}

function DiffDiscountRow({
  discount,
  fmt,
}: {
  discount: DiffDiscountLine;
  fmt: ReturnType<typeof useFormatters>;
}) {
  const t = useTranslations('rfqs');

  return (
    <TableRow
      className={cn(
        '[&>td]:py-1.5',
        discount.changed && 'bg-warning-subtle hover:bg-warning-subtle',
      )}
    >
      <TableCell colSpan={2} className="text-paragraph-xs text-foreground-muted">
        {discount.name}
        {discount.changed ? (
          <span className="ml-2 text-paragraph-xs-medium text-warning-foreground">
            {t('detail.diff.changeTypes.modified')}
          </span>
        ) : null}
      </TableCell>
      <TableCell className="text-right text-paragraph-xs text-foreground-muted tabular-nums">
        −{fmt.currency(discount.amount)}
      </TableCell>
    </TableRow>
  );
}

function DiffPanel({
  title,
  side,
  alignedRows,
  discounts,
  total,
  fmt,
}: {
  title: string;
  side: 'original' | 'requested';
  alignedRows: AlignedRow[];
  discounts: DiffDiscountLine[];
  total: string;
  fmt: ReturnType<typeof useFormatters>;
}) {
  const t = useTranslations('rfqs');

  return (
    <Card className="flex flex-col overflow-hidden">
      <CardHeader className="py-3 border-b border-border">
        <CardTitle className="text-heading-5">{title}</CardTitle>
      </CardHeader>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('detail.diff.columns.product')}</TableHead>
            <TableHead className="text-right">{t('detail.diff.columns.quantity')}</TableHead>
            <TableHead className="text-right">{t('detail.diff.columns.price')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {alignedRows.map((row, i) => (
            <DiffItemCell key={i} item={row[side]} changeType={row.changeType} fmt={fmt} />
          ))}
        </TableBody>
      </Table>

      {discounts.length > 0 && (
        <div className="mt-auto border-t border-border">
          <div className="px-4 pt-3 pb-1">
            <span className="text-paragraph-xs-semibold text-foreground-muted">
              {t('detail.diff.discountsTitle')}
            </span>
          </div>
          <Table>
            <TableBody>
              {discounts.map((d, i) => (
                <DiffDiscountRow key={i} discount={d} fmt={fmt} />
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <div className="border-t border-border px-4 py-3">
        <div className="flex items-center justify-between">
          <span className="text-paragraph-xs-semibold text-foreground-muted">
            {t('detail.items.total')}
          </span>
          <span className="text-paragraph-sm-semibold tabular-nums text-foreground">
            {fmt.currency(total)}
          </span>
        </div>
      </div>
    </Card>
  );
}

export function RfqChangeDiff({ diff }: RfqChangeDiffProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();

  const alignedRows = buildAlignedRows(diff.original.items, diff.requested.items);

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      <DiffPanel
        title={t('detail.diff.originalTitle')}
        side="original"
        alignedRows={alignedRows}
        discounts={diff.original.discounts}
        total={diff.original.total}
        fmt={fmt}
      />
      <DiffPanel
        title={t('detail.diff.requestedTitle')}
        side="requested"
        alignedRows={alignedRows}
        discounts={diff.requested.discounts}
        total={diff.requested.total}
        fmt={fmt}
      />
    </div>
  );
}
