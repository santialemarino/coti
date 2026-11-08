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
import type { SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// The headline figures of the period.
export function ReportKpis({ kpis, fmt }: { kpis: SellerReport['kpis']; fmt: Formatters }) {
  const t = useTranslations('reports');

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
      <SummaryCard icon={InboxIcon} label={t('kpis.rfqs')} value={fmt.value(kpis.rfqsReceived)} />
      <SummaryCard icon={SendIcon} label={t('kpis.sent')} value={fmt.value(kpis.quotesSent)} />
      <SummaryCard
        icon={ChartColumnIcon}
        label={t('kpis.accepted')}
        value={fmt.value(kpis.quotesAccepted)}
        detail={t('kpis.acceptedDetail')}
      />
      <SummaryCard
        icon={TrendingUpIcon}
        label={t('kpis.conversion')}
        value={kpis.conversionRate === null ? t('noDataShort') : fmt.ratePct(kpis.conversionRate)}
        detail={t('conversionDetail', { accepted: kpis.sentAccepted, sent: kpis.quotesSent })}
      />
      <SummaryCard
        icon={WalletIcon}
        label={t('kpis.acceptedAmount')}
        value={fmt.currency(kpis.acceptedAmount)}
        detail={t('kpis.acceptedAmountDetail')}
      />
      <SummaryCard
        icon={ClockIcon}
        label={t('kpis.responseTime')}
        value={formatDuration(kpis.responseTime.averageSeconds)}
        detail={responseDetail}
      />
      <SummaryCard
        icon={HandshakeIcon}
        label={t('kpis.timeToAcceptance')}
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
