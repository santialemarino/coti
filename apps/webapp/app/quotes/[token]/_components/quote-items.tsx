import { getTranslations } from 'next-intl/server';

import { Badge, Separator } from '@repo/ui/components';
import type { QuoteItem } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface QuoteItemsProps {
  items: QuoteItem[];
  currency: string;
}

interface Option {
  key: string;
  label: string;
  name: string;
  price: string;
  unitNote?: string;
  quoted: boolean;
}

/*
 * A list of blocks rather than a table: this is read on a phone first, and four columns at 375px
 * costs the line's own name the width it needs. Figures still line up — each block puts them in
 * one grid whose columns are identical from block to block.
 */
export async function QuoteItems({ items, currency }: QuoteItemsProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  if (items.length === 0) {
    return <p className="text-paragraph text-foreground-muted">{t('empty')}</p>;
  }

  /*
   * The quoted line joins its own alternatives in one list so the prices are compared against each
   * other rather than against the block above. Only alternatives the seller approved ever arrive
   * here — the API drops the rest before the snapshot is frozen. The product code is deliberately
   * absent: the block above already carries it, and it is the corralón's own SKU either way.
   */
  const optionsFor = (item: QuoteItem): Option[] => [
    {
      key: 'quoted',
      label: t('quotedOption'),
      name: item.productName,
      price: fmt.currency(item.unitPrice, currency),
      unitNote: item.unit ? t('perUnit', { unit: item.unit }) : undefined,
      quoted: true,
    },
    ...item.alternatives.map((alternative, index) => ({
      key: `${alternative.code ?? alternative.name}-${index}`,
      label: t('alternativeOption'),
      name: alternative.name,
      price: fmt.currency(alternative.unitPrice, currency),
      unitNote: alternative.unit ? t('perUnit', { unit: alternative.unit }) : undefined,
      quoted: false,
    })),
  ];

  return (
    <section className="flex flex-col gap-y-4">
      <h2 className="text-heading-6 text-foreground">{t('itemsHeading')}</h2>
      <ul className="flex flex-col gap-y-3">
        {items.map((item, index) => (
          <li
            key={`${item.productCode ?? item.productName}-${index}`}
            className="flex flex-col p-4 gap-y-3 bg-card border border-border rounded-1.5xl shadow-e1"
          >
            <div className="flex flex-col gap-y-1">
              <p className="text-paragraph-medium text-foreground">{item.productName}</p>
              {item.productCode ? (
                <p className="text-paragraph-xs text-foreground-subtle tabular-nums">
                  {t('codeLabel', { code: item.productCode })}
                </p>
              ) : null}
              <p className="text-paragraph-sm text-foreground-muted">
                {t('requested', { text: item.requestedDescription })}
              </p>
            </div>

            <dl className="grid grid-cols-3 gap-x-3">
              <div className="flex flex-col gap-y-0.5">
                <dt className="text-paragraph-xs text-foreground-subtle">{t('quantity')}</dt>
                <dd className="text-paragraph-sm text-foreground tabular-nums">
                  {item.unit
                    ? t('quantityWithUnit', {
                        quantity: fmt.value(Number(item.quantity)),
                        unit: item.unit,
                      })
                    : fmt.value(Number(item.quantity))}
                </dd>
              </div>
              <div className="flex flex-col items-end gap-y-0.5">
                <dt className="text-paragraph-xs text-foreground-subtle">{t('unitPrice')}</dt>
                <dd className="text-paragraph-sm text-foreground tabular-nums">
                  {fmt.currency(item.unitPrice, currency)}
                </dd>
              </div>
              <div className="flex flex-col items-end gap-y-0.5">
                <dt className="text-paragraph-xs text-foreground-subtle">{t('subtotal')}</dt>
                <dd className="text-paragraph-sm-medium text-foreground tabular-nums">
                  {fmt.currency(item.subtotal, currency)}
                </dd>
              </div>
            </dl>

            {item.alternatives.length > 0 ? (
              <>
                <Separator />
                <div className="flex flex-col gap-y-2">
                  <p className="text-paragraph-xs-medium text-foreground-muted">
                    {t('alternativesHeading')}
                  </p>
                  <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                    {optionsFor(item).map((option) => (
                      <div
                        key={option.key}
                        className={
                          option.quoted
                            ? 'flex flex-col p-3 gap-y-1.5 bg-accent border border-brand-200 rounded-lg'
                            : 'flex flex-col p-3 gap-y-1.5 bg-sunken border border-border rounded-lg'
                        }
                      >
                        <Badge tone={option.quoted ? 'brand' : 'neutral'} size="sm">
                          {option.label}
                        </Badge>
                        <p className="text-paragraph-sm text-foreground">{option.name}</p>
                        <p className="text-paragraph-sm-medium text-foreground tabular-nums">
                          {option.price}
                          {option.unitNote ? (
                            <span className="text-paragraph-xs text-foreground-muted">
                              {' '}
                              {option.unitNote}
                            </span>
                          ) : null}
                        </p>
                      </div>
                    ))}
                  </div>
                </div>
              </>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
