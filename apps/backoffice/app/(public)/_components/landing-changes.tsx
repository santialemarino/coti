import { ArrowDownIcon, MessageCircleIcon, SparklesIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Badge, Separator } from '@repo/ui/components';
import { LandingSection } from '@/app/(public)/_components/landing-section';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { getFormatters } from '@/lib/i18n/formatters-server';

// The requests a customer answers a quote with, each one Coti turns into a draft for the seller.
const REQUESTS = ['cheaper', 'quality', 'edits'] as const;

// Illustrative figures continuing the hero's quote, written out rather than computed: nothing on
// this page prices anything.
const CHANGE = { quantity: 30, fromPrice: '9800.00', toPrice: '8900.00' } as const;
const TOTAL_BEFORE = '409000.00';
const TOTAL_AFTER = '382000.00';

export async function LandingChanges() {
  const fmt = await getFormatters();
  const t = await getTranslations('landing.changes');

  return (
    <LandingSection
      id="changes"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
      className="bg-card"
    >
      <div className="grid gap-10 lg:grid-cols-[1fr_1.1fr] lg:items-center">
        <ul className="flex flex-col gap-y-6">
          {REQUESTS.map((key, index) => (
            <li key={key} {...revealItem(index + 1, 'flex flex-col gap-y-1')}>
              <h3 className="text-heading-6 text-foreground">{t(`items.${key}.title`)}</h3>
              <p className="text-paragraph-sm text-foreground-muted">
                {t(`items.${key}.description`)}
              </p>
            </li>
          ))}
        </ul>

        <div role="img" aria-label={t('preview.label')} {...revealItem(2, 'flex flex-col gap-y-4')}>
          <div className="flex flex-col max-w-sm p-4 gap-y-2 bg-background border border-border rounded-1.5xl shadow-e2">
            <div className="flex items-center gap-x-2">
              <span className="grid size-7 place-items-center bg-success-subtle rounded-full">
                <MessageCircleIcon aria-hidden="true" className="size-4 text-success-foreground" />
              </span>
              <span className="text-paragraph-sm-semibold text-foreground">
                {t('preview.from')}
              </span>
              <Badge tone="neutral" size="sm" className="ml-auto">
                {t('preview.channel')}
              </Badge>
            </div>
            <p className="text-paragraph-sm text-foreground">{t('preview.message')}</p>
          </div>

          <div className="flex flex-col p-5 gap-y-4 bg-background border border-border rounded-1.5xl shadow-e3 sm:ml-10">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-heading-6 text-foreground">{t('preview.version')}</span>
              <Badge tone="warning" size="sm" dot>
                {t('preview.status')}
              </Badge>
            </div>
            <div className="flex items-start p-3 gap-x-3 bg-sunken rounded-lg">
              <SparklesIcon aria-hidden="true" className="size-4 mt-0.5 shrink-0 text-primary" />
              <div className="flex flex-col gap-y-0.5">
                <span className="text-paragraph-xs-medium text-foreground-muted">
                  {t('preview.interpretationLabel')}
                </span>
                <span className="text-paragraph-sm text-foreground">
                  {t('preview.interpretation')}
                </span>
              </div>
            </div>
            <div className="flex flex-col gap-y-2">
              <Badge tone="warning" size="sm" className="w-fit">
                {t('preview.changed')}
              </Badge>
              <span className="text-paragraph-sm-medium text-foreground">
                {t('preview.line', {
                  from: t('preview.fromProduct'),
                  to: t('preview.toProduct'),
                })}
              </span>
              <span className="text-paragraph-xs text-foreground-muted tabular-nums">
                <s>{fmt.currency(CHANGE.fromPrice)}</s>{' '}
                {t('preview.lineDetail', {
                  quantity: fmt.value(CHANGE.quantity),
                  unit: t('preview.unit'),
                  price: fmt.currency(CHANGE.toPrice),
                })}
              </span>
            </div>
            <Separator />
            <div className="flex items-center justify-between gap-x-4">
              <span className="text-paragraph-sm-medium text-foreground-muted">
                {t('preview.total')}
              </span>
              <span className="flex flex-col items-end">
                <s className="text-paragraph-xs text-foreground-subtle tabular-nums">
                  {fmt.currency(TOTAL_BEFORE)}
                </s>
                <span className="flex items-center gap-x-1 text-heading-5 text-foreground tabular-nums">
                  <ArrowDownIcon aria-hidden="true" className="size-4 text-success-foreground" />
                  {fmt.currency(TOTAL_AFTER)}
                </span>
              </span>
            </div>
            <p className="text-paragraph-xs-medium text-primary">{t('preview.decide')}</p>
          </div>
        </div>
      </div>
    </LandingSection>
  );
}
