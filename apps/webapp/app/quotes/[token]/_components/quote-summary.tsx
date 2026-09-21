import { getTranslations } from 'next-intl/server';

import type { QuoteDiscount, QuoteItem } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface QuoteSummaryProps {
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string;
  currency: string;
}

/*
 * The footer of the items card: subtotal built from the frozen decimal strings, the discounts that
 * trim it, and the backend's authoritative total with the last word. The display never recomputes
 * money the engine already calculated — `total` travels whole from the frozen payload.
 */
export async function QuoteSummary({ items, discounts, total, currency }: QuoteSummaryProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  const itemsSubtotal = items.reduce((sum, item) => sum + Number(item.subtotal), 0);

  return (
    <div className="flex flex-col gap-y-3 border-t border-border px-4 py-4 sm:px-6">
      <div className="flex items-baseline justify-between gap-x-4">
        <p className="text-paragraph-sm text-foreground-muted">{t('subtotal')}</p>
        <p className="text-paragraph-sm text-foreground tabular-nums">
          {fmt.currency(String(itemsSubtotal), currency)}
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
              {/* The formatter owns the sign, so the minus is the locale's and not a typed glyph. */}
              <p className="text-paragraph-sm text-foreground tabular-nums">
                {fmt.currency(`-${discount.amount}`, currency)}
              </p>
            </div>
          ))}
        </div>
      ) : null}

      <div className="flex items-baseline justify-between gap-x-4 border-t border-border pt-3">
        <p className="text-heading-6 text-foreground">{t('totalLabel')}</p>
        <p className="text-heading-4 text-foreground tabular-nums">
          {fmt.currency(total, currency)}
        </p>
      </div>
    </div>
  );
}
