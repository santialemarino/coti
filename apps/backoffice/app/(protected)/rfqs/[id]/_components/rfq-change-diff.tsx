'use client';

import { SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Card, CardHeader, CardTitle } from '@repo/ui/components';
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
  side,
  changeType,
  fmt,
}: {
  item: DiffLineItem | null;
  side: 'original' | 'requested';
  changeType: 'modified' | 'added' | 'removed' | null;
  fmt: ReturnType<typeof useFormatters>;
}) {
  if (!item) {
    return (
      <tr className="bg-muted/30">
        <td className="px-4 py-2.5 text-paragraph-sm text-foreground-subtle">—</td>
        <td className="px-4 py-2.5 text-center text-paragraph-sm text-foreground-subtle">—</td>
        <td className="px-4 py-2.5 text-center text-paragraph-sm text-foreground-subtle">—</td>
      </tr>
    );
  }

  const isRemoved = changeType === 'removed';
  const isAdded = changeType === 'added';
  const isModified = changeType === 'modified';

  const bgClass = isRemoved
    ? 'bg-danger/10'
    : isAdded
      ? 'bg-success/10'
      : isModified
        ? 'bg-warning/10'
        : '';

  const changeLabel = changeType ? (
    <span
      className={`ml-2 text-paragraph-xs font-medium ${
        isRemoved ? 'text-danger' : isAdded ? 'text-success' : 'text-warning'
      }`}
    >
      {isRemoved ? 'eliminado' : isAdded ? 'agregado' : 'modificado'}
    </span>
  ) : null;

  return (
    <tr className={bgClass}>
      <td className="px-4 py-2.5 text-paragraph-sm text-foreground">
        <span
          className={
            isRemoved
              ? 'line-through text-foreground-muted'
              : isAdded
                ? 'font-medium text-foreground'
                : ''
          }
        >
          {item.description}
        </span>
        {changeLabel}
      </td>
      <td className="px-4 py-2.5 text-center tabular-nums text-paragraph-sm">
        {isRemoved ? (
          <span className="text-foreground-muted">—</span>
        ) : (
          <span>
            {fmt.value(Number(item.quantity))} {item.unit ?? ''}
          </span>
        )}
      </td>
      <td className="px-4 py-2.5 text-center tabular-nums text-paragraph-sm-medium">
        {isRemoved ? (
          <span className="text-foreground-muted">—</span>
        ) : item.unit_price != null ? (
          fmt.currency(item.unit_price)
        ) : (
          <span className="text-foreground-subtle">—</span>
        )}
      </td>
    </tr>
  );
}

function DiffDiscountRow({
  discount,
  fmt,
}: {
  discount: DiffDiscountLine;
  fmt: ReturnType<typeof useFormatters>;
}) {
  return (
    <tr className={discount.changed ? 'bg-warning/10' : ''}>
      <td colSpan={2} className="px-4 py-1.5 text-paragraph-xs text-foreground-muted">
        {discount.name}
        {discount.changed && (
          <span className="ml-2 text-paragraph-xs font-medium text-warning">modificado</span>
        )}
      </td>
      <td className="px-4 py-1.5 text-center tabular-nums text-paragraph-xs text-foreground-muted">
        −{fmt.currency(discount.amount)}
      </td>
    </tr>
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

      <div className="overflow-x-auto">
        <table className="w-full border-collapse">
          <thead>
            <tr className="border-y border-border bg-accent/30">
              <th className="px-4 py-2 text-left text-paragraph-xs font-semibold text-foreground-muted">
                {t('detail.diff.columns.product')}
              </th>
              <th className="px-4 py-2 text-center text-paragraph-xs font-semibold text-foreground-muted">
                {t('detail.diff.columns.quantity')}
              </th>
              <th className="px-4 py-2 text-center text-paragraph-xs font-semibold text-foreground-muted">
                {t('detail.diff.columns.price')}
              </th>
            </tr>
          </thead>
          <tbody>
            {alignedRows.map((row, i) => {
              const item = row[side];

              return (
                <DiffItemCell
                  key={i}
                  item={item}
                  side={side}
                  changeType={row.changeType}
                  fmt={fmt}
                />
              );
            })}
          </tbody>
        </table>
      </div>

      {discounts.length > 0 && (
        <div className="mt-auto border-t border-border">
          <div className="px-4 pt-3 pb-1">
            <span className="text-paragraph-xs font-semibold text-foreground-muted">
              {t('detail.diff.discountsTitle')}
            </span>
          </div>
          <table className="w-full border-collapse">
            <tbody>
              {discounts.map((d, i) => (
                <DiffDiscountRow key={i} discount={d} fmt={fmt} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="border-t border-border px-4 py-3">
        <div className="flex items-center justify-between">
          <span className="text-paragraph-xs font-semibold text-foreground-muted">
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
    <div className="flex flex-col gap-y-4">
      {diff.reason && (
        <div className="rounded-md bg-accent/50 px-4 py-3 text-paragraph-sm text-foreground">
          <span className="font-medium text-foreground-muted">{t('detail.diff.reason')}: </span>
          {diff.reason}
        </div>
      )}

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

      <div className="flex justify-center pt-2">
        <Button type="button">
          <SendIcon className="size-4" />
          {t('detail.diff.sendChanges')}
        </Button>
      </div>
    </div>
  );
}
