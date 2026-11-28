import {
  BellRingIcon,
  ChartColumnIcon,
  ClockIcon,
  HandshakeIcon,
  InboxIcon,
  SendIcon,
  TrendingUpIcon,
  WalletIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import { SummaryCard } from '@/app/(protected)/reports/_components/report-summary';
import type { ReportComparison, ReportMetric, SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// The headline figures of the period.
export function ReportKpis({
  kpis,
  fmt,
  comparison,
}: {
  kpis: SellerReport['kpis'];
  fmt: Formatters;
  comparison?: ReportComparison;
}) {
  const t = useTranslations('reports');
  const change = (metric: ReportMetric) =>
    comparison ? <ReportChange metric={metric} comparison={comparison} fmt={fmt} /> : undefined;

  const formatDuration = (seconds: number | null) => {
    if (seconds === null) return t('noDataShort');
    if (seconds < 60) return t('duration.seconds', { count: Math.round(seconds) });
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return t('duration.minutes', { count: minutes });
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    // Past two days the minutes are noise and hours stop reading as a span; days carry it.
    if (hours >= 48) {
      const days = Math.floor(hours / 24);
      const dayHours = hours % 24;
      return dayHours === 0
        ? t('duration.days', { count: days })
        : t('duration.daysHours', { days, hours: dayHours });
    }
    return rest === 0
      ? t('duration.hours', { count: hours })
      : t('duration.hoursMinutes', { hours, minutes: rest });
  };
  const responseDetail =
    kpis.responseTime.sample > 0 && kpis.responseTime.medianSeconds !== null
      ? t('responseDetail', {
          median: formatDuration(kpis.responseTime.medianSeconds),
          count: kpis.responseTime.sample,
        })
      : t('noDataShort');

  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <SummaryCard
        icon={InboxIcon}
        label={t('kpis.rfqs')}
        value={fmt.value(kpis.rfqsReceived)}
        comparison={change('rfqsReceived')}
      />
      <SummaryCard
        icon={SendIcon}
        label={t('kpis.sent')}
        value={fmt.value(kpis.quotesSent)}
        comparison={change('quotesSent')}
      />
      <SummaryCard
        icon={ChartColumnIcon}
        label={t('kpis.accepted')}
        comparison={change('quotesAccepted')}
        value={fmt.value(kpis.quotesAccepted)}
        detail={t('kpis.acceptedDetail')}
      />
      <SummaryCard
        icon={TrendingUpIcon}
        label={t('kpis.conversion')}
        comparison={change('conversionRate')}
        value={kpis.conversionRate === null ? t('noDataShort') : fmt.ratePct(kpis.conversionRate)}
        detail={t('conversionDetail', { accepted: kpis.sentAccepted, sent: kpis.quotesSent })}
      />
      <SummaryCard
        icon={WalletIcon}
        label={t('kpis.acceptedAmount')}
        value={
          kpis.acceptedAmounts.length === 0
            ? t('noDataShort')
            : kpis.acceptedAmounts.map(({ currency, amount }) => (
                <span key={currency} className="block">
                  {currency} {fmt.currency(amount, currency)}
                </span>
              ))
        }
        detail={t('kpis.acceptedAmountDetail')}
      />
      <SummaryCard
        icon={ClockIcon}
        label={t('kpis.responseTime')}
        comparison={change('responseTime')}
        value={formatDuration(kpis.responseTime.averageSeconds)}
        detail={responseDetail}
      />
      <SummaryCard
        icon={HandshakeIcon}
        label={t('kpis.timeToAcceptance')}
        comparison={change('timeToAcceptance')}
        value={formatDuration(kpis.timeToAcceptance.averageSeconds)}
        detail={
          kpis.timeToAcceptance.sample > 0 && kpis.timeToAcceptance.medianSeconds !== null
            ? t('timeToAcceptanceDetail', {
                median: formatDuration(kpis.timeToAcceptance.medianSeconds),
                count: kpis.timeToAcceptance.sample,
              })
            : t('noDataShort')
        }
      />
      <SummaryCard
        icon={BellRingIcon}
        label={t('kpis.followUpPending')}
        value={fmt.value(kpis.followUpPending)}
        detail={t('kpis.followUpPendingDetail')}
      />
    </div>
  );
}

function ReportChange({
  metric,
  comparison,
  fmt,
}: {
  metric: ReportMetric;
  comparison: ReportComparison;
  fmt: Formatters;
}) {
  const t = useTranslations('reports.comparison');
  const value = comparison.metrics[metric];
  if (!value) return null;
  const duration = metric === 'responseTime' || metric === 'timeToAcceptance';
  const previous =
    value.previous === null
      ? t('noData')
      : value.percentagePoints
        ? fmt.ratePct(Number(value.previous))
        : duration
          ? t('seconds', { value: fmt.value(Number(value.previous)) })
          : fmt.value(Number(value.previous));
  const delta =
    value.delta === null
      ? t('noData')
      : value.percentagePoints
        ? t('points', { value: fmt.signedValue(Number(value.delta)) })
        : duration
          ? t('seconds', { value: fmt.signedValue(Number(value.delta)) })
          : fmt.signedValue(Number(value.delta));
  const percentage =
    value.percentage === null
      ? t('noBase')
      : t('percent', { value: fmt.signedValue(Number(value.percentage)) });
  const improvement =
    value.delta !== null && Number(value.delta) !== 0 ? Number(value.delta) > 0 !== duration : null;
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-paragraph-xs tabular-nums"
      title={t('period', {
        from: fmt.dateNumeric(comparison.range.from),
        to: fmt.dateNumeric(comparison.range.to),
      })}
    >
      <p className="text-foreground-muted">{t('previous', { value: previous })}</p>
      <p
        className={
          improvement === null
            ? 'text-foreground-muted'
            : improvement
              ? 'text-success-foreground'
              : 'text-danger-foreground'
        }
      >
        {t('change', { delta, percentage: value.percentagePoints ? t('rate') : percentage })}
      </p>
    </div>
  );
}
