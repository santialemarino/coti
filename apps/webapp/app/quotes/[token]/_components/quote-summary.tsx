import { DownloadIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Separator } from '@repo/ui/components';
import type { QuoteDiscount } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface QuoteSummaryProps {
  discounts: QuoteDiscount[];
  total: string;
  currency: string;
  expiresAt: string;
  validityNote: string;
  pdfUrl?: string;
}

export async function QuoteSummary({
  discounts,
  total,
  currency,
  expiresAt,
  validityNote,
  pdfUrl,
}: QuoteSummaryProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  return (
    <section className="flex flex-col p-4 gap-y-3 bg-card border border-border rounded-1.5xl shadow-e1">
      {discounts.length > 0 ? (
        <>
          <h2 className="text-heading-6 text-foreground">{t('discountsHeading')}</h2>
          <dl className="flex flex-col gap-y-1.5">
            {discounts.map((discount, index) => (
              <div
                key={`${discount.description}-${index}`}
                className="flex items-baseline justify-between gap-x-4"
              >
                <dt className="text-paragraph-sm text-foreground-muted">{discount.description}</dt>
                <dd className="text-paragraph-sm text-foreground tabular-nums">
                  −{fmt.currency(discount.amount, currency)}
                </dd>
              </div>
            ))}
          </dl>
          <Separator />
        </>
      ) : null}

      <div className="flex items-baseline justify-between gap-x-4">
        <p className="text-heading-6 text-foreground">{t('totalLabel')}</p>
        <p className="text-heading-4 text-foreground tabular-nums">
          {fmt.currency(total, currency)}
        </p>
      </div>

      <p className="text-paragraph-sm text-foreground-muted">
        {t('validUntil', { date: fmt.date(expiresAt) })}
      </p>
      <p className="text-paragraph-xs text-foreground-subtle">{validityNote}</p>

      {pdfUrl ? (
        <Button asChild variant="outline" className="mt-1 self-start">
          <a href={pdfUrl} target="_blank" rel="noopener noreferrer">
            <DownloadIcon />
            {t('downloadPdf')}
          </a>
        </Button>
      ) : null}
    </section>
  );
}
