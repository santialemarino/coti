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

  // Orders built by hand have no AI reading but can still carry unmatched lines worth showing.
  const hasReading = aiMatch.total > 0;

  return (
    <ReportSection
      title={t('ai.title')}
      empty={!hasReading && aiMatch.versionLines === 0 ? t('ai.empty') : undefined}
      className={className}
    >
      <div className="flex flex-col gap-y-4">
        {hasReading ? (
          <>
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
          </>
        ) : null}
        {aiMatch.versionLines > 0 ? (
          <p className="text-paragraph-xs text-foreground-muted">
            {t('ai.versionUnmatched', {
              unmatched: aiMatch.versionUnmatched,
              total: fmt.value(aiMatch.versionLines),
            })}
          </p>
        ) : null}
      </div>
    </ReportSection>
  );
}
