import { useTranslations } from 'next-intl';

import { ReportSection } from '@/app/(protected)/reports/_components/report-section';
import type { SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// How sellers treated the conversation assistant's proposals: the share they approved untouched.
export function ReportAssistant({
  assistant,
  fmt,
  className,
}: {
  assistant: SellerReport['assistant'];
  fmt: Formatters;
  className?: string;
}) {
  const t = useTranslations('reports.assistant');
  const outcomes = [
    { key: 'approvedAsIs', value: assistant.approvedAsIs },
    { key: 'edited', value: assistant.edited },
    { key: 'rejected', value: assistant.rejected },
    { key: 'manualOverride', value: assistant.manualOverride },
    { key: 'pending', value: assistant.pending },
  ] as const;

  return (
    <ReportSection
      title={t('title')}
      description={t('description')}
      empty={assistant.proposals === 0 ? t('empty') : undefined}
      className={className}
    >
      <div className="flex flex-col gap-6 sm:flex-row sm:items-center">
        <div className="shrink-0">
          <p className="text-heading-4 tabular-nums text-foreground">
            {assistant.approvalRate === null ? '—' : fmt.ratePct(assistant.approvalRate)}
          </p>
          <p className="text-paragraph-xs text-foreground-muted">
            {t('rate', { approved: assistant.approvedAsIs, total: fmt.value(assistant.proposals) })}
          </p>
        </div>
        <dl className="grid flex-1 grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3 lg:grid-cols-5">
          {outcomes.map((outcome) => (
            <div key={outcome.key} className="flex flex-col gap-y-0.5">
              <dt className="text-paragraph-xs text-foreground-muted">
                {t(`outcomes.${outcome.key}`)}
              </dt>
              <dd className="text-paragraph-sm-semibold tabular-nums text-foreground">
                {fmt.value(outcome.value)}
              </dd>
            </div>
          ))}
        </dl>
      </div>
    </ReportSection>
  );
}
