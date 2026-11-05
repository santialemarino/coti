import { useTranslations } from 'next-intl';

import { ConfidenceBar } from '@/app/(protected)/reports/_components/report-charts';
import { ReportSection } from '@/app/(protected)/reports/_components/report-section';
import type { SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// How much of each order the latest AI reading matched, and how settled those matches were.
export function ReportAIMatch({
  aiMatch,
  fmt,
  className,
}: {
  aiMatch: SellerReport['aiMatch'];
  fmt: Formatters;
  className?: string;
}) {
  const t = useTranslations('reports');

  return (
    <ReportSection
      title={t('ai.title')}
      empty={aiMatch.total === 0 ? t('ai.empty') : undefined}
      className={className}
    >
      <div className="flex flex-col gap-y-4">
        <div>
          <p className="text-heading-4 tabular-nums text-foreground">
            {aiMatch.rate === null ? t('noDataShort') : fmt.ratePct(aiMatch.rate)}
          </p>
          <p className="text-paragraph-xs text-foreground-muted">
            {t('ai.matched', { matched: aiMatch.matched, total: fmt.value(aiMatch.total) })}
          </p>
        </div>
        <ConfidenceBar
          low={aiMatch.confidence.low}
          medium={aiMatch.confidence.medium}
          high={aiMatch.confidence.high}
          labels={{ low: t('ai.low'), medium: t('ai.medium'), high: t('ai.high') }}
          formatValue={fmt.value}
        />
        <p className="text-paragraph-xs text-foreground-muted">
          {t('ai.versionUnmatched', {
            unmatched: aiMatch.versionUnmatched,
            total: fmt.value(aiMatch.versionLines),
          })}
        </p>
      </div>
    </ReportSection>
  );
}
