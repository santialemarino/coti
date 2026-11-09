'use client';

import { useTranslations } from 'next-intl';

import type { QuoteDiscount, QuoteItem } from '@/lib/api/public-quotes';
import { useFormatters } from '@/lib/i18n/formatters';

interface QuoteSummaryProps {
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string;
  currency: string;
  subtotal?: string;
  isCalculating?: boolean;
}

/* The backend is authoritative for selected-option pricing; the initial frozen payload has no subtotal field. */
export function QuoteSummary({
  items,
  discounts,
  total,
  currency,
  subtotal,
  isCalculating = false,
}: QuoteSummaryProps) {
  const fmt = useFormatters();
  const t = useTranslations('quote');
  const payloadSubtotal = items.reduce((sum, item) => sum + Number(item.subtotal), 0);

  return (
    <div className="flex flex-col gap-y-3 border-t border-border-strong px-4 py-4 sm:px-6">
      <div className="flex items-baseline justify-between gap-x-4">
        <p className="text-paragraph-sm text-foreground-muted">{t('subtotal')}</p>
        <p className="text-paragraph-sm text-foreground tabular-nums">
          {fmt.currency(subtotal ?? String(payloadSubtotal), currency)}
        </p>
      </div>

      {discounts.length > 0 ? (
        <div className="flex flex-col gap-y-1.5">
          {discounts.map((discount, index) => (
            <div
              key={`${discount.description}-${index}`}
              className="flex items-baseline justify-between gap-x-4"
            >
              <p className="text-paragraph-sm text-foreground-muted">{discount.description}</p>
              <p className="text-paragraph-sm text-foreground tabular-nums">
                {fmt.currency(`-${discount.amount}`, currency)}
              </p>
            </div>
          ))}
        </div>
      ) : null}

      <div className="flex items-baseline justify-between gap-x-4 border-t border-border pt-3">
        <div className="flex flex-col gap-y-0.5">
          <p className="text-heading-6 text-foreground">{t('totalLabel')}</p>
          <p aria-live="polite" className="text-paragraph-xs text-foreground-muted">
            {isCalculating ? t('recalculating') : null}
          </p>
        </div>
        <p className="text-heading-4 text-foreground tabular-nums">
          {fmt.currency(total, currency)}
        </p>
      </div>
    </div>
  );
}
