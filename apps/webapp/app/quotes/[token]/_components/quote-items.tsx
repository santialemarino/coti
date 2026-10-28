'use client';

import { Fragment, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { CircleCheckIcon, CircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
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
import type {
  PublicQuoteAlternativeSelection,
  PublicQuoteSelectionPreview,
  QuoteDiscount,
  QuoteItem,
} from '@/lib/api/public-quotes';
import { useFormatters } from '@/lib/i18n/formatters';
import { useQuoteSelections } from './quote-selection-context';
import { QuoteSummary } from './quote-summary';

interface QuoteItemsProps {
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string;
  currency: string;
  token?: string;
  disabled?: boolean;
}

interface QuotePricing {
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string;
  subtotal?: string;
}

/* Options stay next to their quoted material; the final amount comes only from the pricing preview. */
export function QuoteItems({
  items,
  discounts,
  total,
  currency,
  token,
  disabled = false,
}: QuoteItemsProps) {
  const fmt = useFormatters();
  const t = useTranslations('quote');
  const { selections, select, selectedAlternativeId } = useQuoteSelections();
  const basePricing = useMemo<QuotePricing>(
    () => ({ items, discounts, total }),
    [discounts, items, total],
  );
  const [pricing, setPricing] = useState(basePricing);
  const [isCalculating, setIsCalculating] = useState(false);
  const [previewFailed, setPreviewFailed] = useState(false);
  const requestID = useRef(0);

  useEffect(() => {
    const currentRequest = ++requestID.current;
    if (selections.length === 0 || !token || disabled) {
      setPricing(basePricing);
      setIsCalculating(false);
      setPreviewFailed(false);
      return;
    }
    setIsCalculating(true);
    setPreviewFailed(false);
    void requestSelectionPreview(token, selections)
      .then((preview) => {
        if (currentRequest !== requestID.current) return;
        setPricing({
          items: preview.quote.items,
          discounts: preview.quote.discounts,
          total: preview.quote.total,
          subtotal: preview.subtotal,
        });
      })
      .catch(() => {
        if (currentRequest !== requestID.current) return;
        setPricing(basePricing);
        setPreviewFailed(true);
      })
      .finally(() => {
        if (currentRequest === requestID.current) setIsCalculating(false);
      });
  }, [basePricing, disabled, selections, token]);

  if (items.length === 0) {
    return <p className="text-paragraph text-foreground-muted">{t('empty')}</p>;
  }

  const quantityOf = (item: QuoteItem) => fmt.value(Number(item.quantity));
  const chooseAlternative = (itemId: string, alternativeId: string, selected: boolean) => {
    if (disabled) return;
    select(itemId, selected ? undefined : alternativeId);
  };
  const onAlternativeKeyDown = (
    event: KeyboardEvent<HTMLTableRowElement>,
    itemId: string,
    alternativeId: string,
    selected: boolean,
  ) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    chooseAlternative(itemId, alternativeId, selected);
  };

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <CardHeader className="flex-row items-center justify-between py-4">
        <CardTitle className="text-heading-5">
          {t('itemsHeading')}
          <span className="ml-2 text-paragraph-sm text-foreground-muted">
            {t('itemCount', { count: items.length })}
          </span>
        </CardTitle>
      </CardHeader>

      <div data-testid="items-table-desktop" className="hidden md:block">
        <Table figures="end" className="[&_th]:h-10 [&_td]:py-3">
          <TableHeader>
            <TableRow>
              <TableHead kind="index">#</TableHead>
              <TableHead kind="text">{t('product')}</TableHead>
              <TableHead kind="short">{t('code')}</TableHead>
              <TableHead kind="quantity">{t('quantity')}</TableHead>
              <TableHead kind="money">{t('unitPrice')}</TableHead>
              <TableHead kind="money">{t('subtotal')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item, index) => (
              <Fragment key={item.id}>
                <TableRow>
                  <TableCell kind="index" className="text-foreground-subtle">
                    {index + 1}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col items-start gap-y-0.5">
                      <span className="text-paragraph-sm-medium text-foreground">
                        {item.productName}
                      </span>
                      <span className="text-paragraph-xs text-foreground-subtle">
                        {t('requested', { text: item.requestedDescription })}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell kind="short" className="text-paragraph-xs text-foreground-muted">
                    {item.productCode ?? '—'}
                  </TableCell>
                  <TableCell kind="quantity" className="text-foreground">
                    {quantityOf(item)}
                    {item.unit ? <span className="text-foreground-muted"> {item.unit}</span> : null}
                  </TableCell>
                  <TableCell kind="money" className="text-foreground">
                    {fmt.currency(item.unitPrice, currency)}
                  </TableCell>
                  <TableCell kind="money" className="text-paragraph-sm-medium text-foreground">
                    {fmt.currency(item.subtotal, currency)}
                  </TableCell>
                </TableRow>
                {item.alternatives.map((alternative) => {
                  const selected = selectedAlternativeId(item.id) === alternative.id;
                  const selectable = alternative.selectable && !disabled;
                  return (
                    <TableRow
                      key={alternative.id}
                      interactive={selectable}
                      tabIndex={selectable ? 0 : undefined}
                      role={selectable ? 'button' : undefined}
                      aria-pressed={selectable ? selected : undefined}
                      aria-label={
                        selectable
                          ? t(selected ? 'restoreQuotedOption' : 'chooseAlternative', {
                              product: alternative.name,
                            })
                          : undefined
                      }
                      data-state={selected ? 'selected' : undefined}
                      className={
                        selected
                          ? 'border-primary bg-primary-subtle hover:bg-primary-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary'
                          : selectable
                            ? 'bg-muted/30 hover:bg-primary-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary'
                            : 'bg-muted/30'
                      }
                      onClick={() => chooseAlternative(item.id, alternative.id, selected)}
                      onKeyDown={(event) =>
                        onAlternativeKeyDown(event, item.id, alternative.id, selected)
                      }
                    >
                      <TableCell kind="index" className="text-primary">
                        {alternative.selectable ? (
                          selected ? (
                            <CircleCheckIcon aria-hidden="true" className="size-4" />
                          ) : (
                            <CircleIcon aria-hidden="true" className="size-4" />
                          )
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-x-2">
                          <Badge tone={selected ? 'brand' : 'neutral'} size="sm">
                            {alternativeLabel(t, alternative.recommendationType)}
                          </Badge>
                          <span className="text-paragraph-sm text-foreground-muted">
                            {alternative.name}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell kind="short" className="text-paragraph-xs text-foreground-muted">
                        {alternative.code ?? '—'}
                      </TableCell>
                      <TableCell />
                      <TableCell kind="money" className="text-foreground-muted">
                        {fmt.currency(alternative.unitPrice, currency)}
                        {alternative.unit ? (
                          <span className="ml-1 text-paragraph-xs text-foreground-subtle">
                            {t('perUnit', { unit: alternative.unit })}
                          </span>
                        ) : null}
                      </TableCell>
                      <TableCell />
                    </TableRow>
                  );
                })}
              </Fragment>
            ))}
          </TableBody>
        </Table>
      </div>

      <ul data-testid="items-list-mobile" className="divide-y divide-border md:hidden">
        {items.map((item) => (
          <li key={item.id} className="flex flex-col gap-y-2.5 px-4 py-4 sm:px-6">
            <div className="flex items-start justify-between gap-x-3">
              <div className="flex min-w-0 flex-col gap-y-0.5">
                <p className="text-paragraph-sm-medium text-foreground">{item.productName}</p>
                {item.productCode ? (
                  <p className="text-paragraph-xs text-foreground-muted tabular-nums">
                    {t('codeLabel', { code: item.productCode })}
                  </p>
                ) : null}
              </div>
              <p className="shrink-0 text-paragraph-sm-medium text-foreground tabular-nums">
                {fmt.currency(item.subtotal, currency)}
              </p>
            </div>
            <p className="text-paragraph-xs text-foreground-subtle">
              {t('requested', { text: item.requestedDescription })}
            </p>
            <dl className="grid grid-cols-2 gap-x-3">
              <div className="flex flex-col gap-y-0.5">
                <dt className="text-paragraph-xs text-foreground-subtle">{t('quantity')}</dt>
                <dd className="text-paragraph-sm text-foreground tabular-nums">
                  {item.unit
                    ? t('quantityWithUnit', { quantity: quantityOf(item), unit: item.unit })
                    : quantityOf(item)}
                </dd>
              </div>
              <div className="flex flex-col items-end gap-y-0.5">
                <dt className="text-paragraph-xs text-foreground-subtle">{t('unitPrice')}</dt>
                <dd className="text-paragraph-sm text-foreground tabular-nums">
                  {fmt.currency(item.unitPrice, currency)}
                </dd>
              </div>
            </dl>

            {item.alternatives.length > 0 ? (
              <div className="flex flex-col gap-y-1.5 border-t border-border pt-2.5">
                <p className="text-paragraph-xs-medium text-foreground-muted">
                  {t('alternativesHeading')}
                </p>
                {item.alternatives.map((alternative) => {
                  const selected = selectedAlternativeId(item.id) === alternative.id;
                  const selectable = alternative.selectable && !disabled;
                  return (
                    <button
                      key={alternative.id}
                      type="button"
                      disabled={!selectable}
                      aria-pressed={selected}
                      className={
                        selected
                          ? 'flex w-full items-center justify-between gap-x-3 rounded-md border border-primary bg-primary-subtle px-3 py-2 text-left transition-[color,background-color,border-color,box-shadow] duration-200 ease-out-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background'
                          : selectable
                            ? 'flex w-full items-center justify-between gap-x-3 rounded-md border border-transparent bg-muted/30 px-3 py-2 text-left transition-[color,background-color,border-color,box-shadow] duration-200 ease-out-soft hover:bg-primary-subtle active:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background'
                            : 'flex w-full items-center justify-between gap-x-3 rounded-md border border-transparent bg-muted/30 px-3 py-2 text-left'
                      }
                      onClick={() => chooseAlternative(item.id, alternative.id, selected)}
                    >
                      <span className="flex min-w-0 items-center gap-x-2">
                        {alternative.selectable ? (
                          selected ? (
                            <CircleCheckIcon
                              aria-hidden="true"
                              className="size-4 shrink-0 text-primary"
                            />
                          ) : (
                            <CircleIcon
                              aria-hidden="true"
                              className="size-4 shrink-0 text-foreground-subtle"
                            />
                          )
                        ) : null}
                        <span className="flex min-w-0 items-center gap-x-2">
                          <Badge tone={selected ? 'brand' : 'neutral'} size="sm">
                            {alternativeLabel(t, alternative.recommendationType)}
                          </Badge>
                          <span className="truncate text-paragraph-sm text-foreground-muted">
                            {alternative.name}
                          </span>
                        </span>
                      </span>
                      <span className="shrink-0 text-paragraph-sm text-foreground-muted tabular-nums">
                        {fmt.currency(alternative.unitPrice, currency)}
                        {alternative.unit ? (
                          <span className="ml-1 text-paragraph-xs text-foreground-subtle">
                            {t('perUnit', { unit: alternative.unit })}
                          </span>
                        ) : null}
                      </span>
                    </button>
                  );
                })}
              </div>
            ) : null}
          </li>
        ))}
      </ul>

      {previewFailed ? (
        <p role="status" className="px-4 pt-3 text-paragraph-sm text-danger-foreground sm:px-6">
          {t('recalculationError')}
        </p>
      ) : null}
      <QuoteSummary
        items={pricing.items}
        discounts={pricing.discounts}
        total={pricing.total}
        currency={currency}
        subtotal={pricing.subtotal}
        isCalculating={isCalculating}
      />
    </Card>
  );
}

async function requestSelectionPreview(
  token: string,
  selections: PublicQuoteAlternativeSelection[],
): Promise<PublicQuoteSelectionPreview> {
  const response = await fetch('/api/public-quote-preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      token,
      selections: selections.map((selection) => ({
        item_id: selection.itemId,
        alternative_id: selection.alternativeId,
      })),
    }),
  });
  if (!response.ok) throw new Error(`selection preview failed with ${response.status}`);
  return (await response.json()) as PublicQuoteSelectionPreview;
}

function alternativeLabel(
  t: ReturnType<typeof useTranslations>,
  type: QuoteItem['alternatives'][number]['recommendationType'],
): string {
  if (type === 'ECONOMY') return t('options.economy');
  if (type === 'PREMIUM') return t('options.premium');
  return t('alternativeOption');
}
