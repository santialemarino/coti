import { MessageCircleIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Badge, Separator } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { getFormatters } from '@/lib/i18n/formatters-server';

// Illustrative figures for the demo quote, written out rather than computed: nothing on this page
// prices anything.
const LINES = [
  { key: 'cement', quantity: 30, unitPrice: '9800.00', amount: '294000.00', match: 'matched' },
  { key: 'sand', quantity: 2, unitPrice: '42000.00', amount: '84000.00', match: 'matched' },
  { key: 'brick', quantity: 50, unitPrice: '620.00', amount: '31000.00', match: 'review' },
] as const;
const TOTAL = '409000.00';

interface ProductPreviewProps {
  className?: string;
}

// A request as it arrives and the quote Coti drafts from it, drawn with the app's own components.
export async function ProductPreview({ className }: ProductPreviewProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('landing.preview');

  return (
    <div role="img" aria-label={t('label')} className={cn('flex flex-col gap-y-4', className)}>
      <div className="flex flex-col max-w-sm p-4 gap-y-2 bg-card border border-border rounded-1.5xl shadow-e2">
        <div className="flex items-center gap-x-2">
          <span className="grid size-7 place-items-center bg-success-subtle rounded-full">
            <MessageCircleIcon aria-hidden="true" className="size-4 text-success-foreground" />
          </span>
          <span className="text-paragraph-sm-semibold text-foreground">{t('from')}</span>
          <Badge tone="neutral" size="sm" className="ml-auto">
            {t('channel')}
          </Badge>
        </div>
        <p className="text-paragraph-sm text-foreground">{t('message')}</p>
      </div>

      <div className="flex flex-col p-5 gap-y-4 bg-card border border-border rounded-1.5xl shadow-e3 sm:ml-10">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-heading-6 text-foreground">{t('quote')}</span>
          <Badge tone="brand" size="sm" dot>
            {t('status')}
          </Badge>
        </div>
        <ul className="flex flex-col gap-y-3">
          {LINES.map((line) => (
            <li key={line.key} className="grid grid-cols-[1fr_auto] items-start gap-x-4">
              <div className="flex flex-col min-w-0 gap-y-1">
                <span className="text-paragraph-sm-medium text-foreground">
                  {t(`lines.${line.key}.name`)}
                </span>
                <span className="text-paragraph-xs text-foreground-muted tabular-nums">
                  {t('quantity', {
                    quantity: fmt.value(line.quantity),
                    unit: t(`lines.${line.key}.unit`),
                    price: fmt.currency(line.unitPrice),
                  })}
                </span>
                <Badge
                  tone={line.match === 'matched' ? 'success' : 'warning'}
                  size="sm"
                  dot
                  className="w-fit"
                >
                  {t(`match.${line.match}`)}
                </Badge>
              </div>
              <span className="text-paragraph-sm-medium text-foreground tabular-nums">
                {fmt.currency(line.amount)}
              </span>
            </li>
          ))}
        </ul>
        <Separator />
        <div className="flex items-center justify-between gap-x-4">
          <span className="text-paragraph-sm-medium text-foreground-muted">{t('total')}</span>
          <span className="text-heading-5 text-foreground tabular-nums">{fmt.currency(TOTAL)}</span>
        </div>
      </div>
    </div>
  );
}
