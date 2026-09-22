import { Fragment } from 'react';
import { getTranslations } from 'next-intl/server';

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
import type { QuoteDiscount, QuoteItem } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { QuoteSummary } from './quote-summary';

interface QuoteItemsProps {
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string;
  currency: string;
}

/*
 * The items read as one document rather than a stack of cards: a shared heading, a table on
 * desktop and compact hairline-divided blocks on a phone, with the financial summary closing the
 * card. The same data is rendered twice, once per breakpoint, because a phone can never fit six
 * columns — each variant is `display:none` at the other breakpoint, so only one is in the
 * accessibility tree at a time.
 */
export async function QuoteItems({ items, discounts, total, currency }: QuoteItemsProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  if (items.length === 0) {
    return <p className="text-paragraph text-foreground-muted">{t('empty')}</p>;
  }

  const quantityOf = (item: QuoteItem) => fmt.value(Number(item.quantity));

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
        <Table className="[&_th]:h-10 [&_td]:py-3">
          <TableHeader>
            {/*
             * The same alignment rule the backoffice follows: copy reads from a common left edge,
             * figures line up on the right with tabular figures.
             */}
            <TableRow>
              <TableHead className="w-8 text-right">#</TableHead>
              <TableHead>{t('product')}</TableHead>
              <TableHead>{t('code')}</TableHead>
              <TableHead className="text-right">{t('quantity')}</TableHead>
              <TableHead className="text-right">{t('unit')}</TableHead>
              <TableHead className="text-right">{t('unitPrice')}</TableHead>
              <TableHead className="text-right">{t('subtotal')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item, index) => (
              <Fragment key={`${item.productCode ?? item.productName}-${index}`}>
                <TableRow>
                  <TableCell className="text-right tabular-nums text-foreground-subtle">
                    {index + 1}
                  </TableCell>
                  <TableCell className="max-w-md">
                    <div className="flex flex-col items-start gap-y-0.5">
                      <span className="text-paragraph-sm-medium text-foreground">
                        {item.productName}
                      </span>
                      <span className="text-paragraph-xs text-foreground-subtle">
                        {t('requested', { text: item.requestedDescription })}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-paragraph-xs text-foreground-muted tabular-nums">
                    {item.productCode ?? '—'}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-foreground">
                    {quantityOf(item)}
                  </TableCell>
                  <TableCell className="text-right text-paragraph-sm text-foreground-muted">
                    {item.unit ?? '—'}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-foreground">
                    {fmt.currency(item.unitPrice, currency)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-paragraph-sm-medium text-foreground">
                    {fmt.currency(item.subtotal, currency)}
                  </TableCell>
                </TableRow>
                {/*
                 * An approved alternative sits under the line it offers instead of a different one,
                 * so the quoted price and the alternative are read in the same column and compared
                 * directly. The API only ever freezes alternatives the seller approved.
                 */}
                {item.alternatives.map((alternative, altIndex) => (
                  <TableRow
                    key={`${alternative.code ?? alternative.name}-${altIndex}`}
                    className="bg-muted/30 hover:bg-muted/40"
                  >
                    <TableCell />
                    <TableCell>
                      <div className="flex items-center gap-x-2">
                        <Badge tone="neutral" size="sm">
                          {t('alternativeOption')}
                        </Badge>
                        <span className="text-paragraph-sm text-foreground-muted">
                          {alternative.name}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-paragraph-xs text-foreground-muted tabular-nums">
                      {alternative.code ?? '—'}
                    </TableCell>
                    <TableCell />
                    <TableCell />
                    <TableCell className="text-right tabular-nums text-foreground-muted">
                      {fmt.currency(alternative.unitPrice, currency)}
                      {alternative.unit ? (
                        <span className="ml-1 text-paragraph-xs text-foreground-subtle">
                          {t('perUnit', { unit: alternative.unit })}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell />
                  </TableRow>
                ))}
              </Fragment>
            ))}
          </TableBody>
        </Table>
      </div>

      <ul data-testid="items-list-mobile" className="md:hidden divide-y divide-border">
        {items.map((item, index) => (
          <li
            key={`${item.productCode ?? item.productName}-${index}`}
            className="flex flex-col gap-y-2.5 px-4 py-4 sm:px-6"
          >
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
                {item.alternatives.map((alternative, altIndex) => (
                  <div
                    key={`${alternative.code ?? alternative.name}-${altIndex}`}
                    className="flex items-center justify-between gap-x-3"
                  >
                    <span className="flex min-w-0 items-center gap-x-2">
                      <Badge tone="neutral" size="sm">
                        {t('alternativeOption')}
                      </Badge>
                      <span className="truncate text-paragraph-sm text-foreground-muted">
                        {alternative.name}
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
                  </div>
                ))}
              </div>
            ) : null}
          </li>
        ))}
      </ul>

      <QuoteSummary items={items} discounts={discounts} total={total} currency={currency} />
    </Card>
  );
}
