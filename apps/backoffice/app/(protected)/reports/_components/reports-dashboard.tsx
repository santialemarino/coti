import { ChartColumnIcon, Clock3Icon, PackageIcon, ShoppingBagIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  Input,
} from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ROUTES } from '@/config/routes';
import type { SellerReport } from '@/lib/api/reports';
import { getFormatters } from '@/lib/i18n/formatters-server';

const STATUS_CHART_RADIUS = 42;
const STATUS_CHART_CIRCUMFERENCE = 2 * Math.PI * STATUS_CHART_RADIUS;
const STATUS_CHART_COLOUR: Record<string, { stroke: string; marker: string }> = {
  DRAFT: { stroke: 'text-status-draft', marker: 'bg-status-draft' },
  QUOTED: { stroke: 'text-status-quoted', marker: 'bg-status-quoted' },
  SENT: { stroke: 'text-status-sent', marker: 'bg-status-sent' },
  CHANGE_REQUESTED: {
    stroke: 'text-status-change-requested',
    marker: 'bg-status-change-requested',
  },
  ACCEPTED: { stroke: 'text-status-accepted', marker: 'bg-status-accepted' },
  REJECTED: { stroke: 'text-status-rejected', marker: 'bg-status-rejected' },
};

interface ReportsDashboardProps {
  report: SellerReport;
  dateFrom?: string;
  dateTo?: string;
}

interface MetricCardProps {
  title: string;
  value: string;
  description: string;
  icon: typeof ShoppingBagIcon;
}

function MetricCard({ title, value, description, icon: Icon }: MetricCardProps) {
  return (
    <Card className="gap-y-4">
      <CardHeader className="flex-row items-start justify-between">
        <div className="flex flex-col gap-y-1">
          <CardDescription>{title}</CardDescription>
          <p className="text-heading-3 tabular-nums">{value}</p>
        </div>
        <span className="flex size-10 items-center justify-center rounded-lg bg-accent text-accent-foreground">
          <Icon aria-hidden="true" className="size-5" />
        </span>
      </CardHeader>
      <CardContent>
        <p className="text-paragraph-sm text-foreground-muted">{description}</p>
      </CardContent>
    </Card>
  );
}

export async function ReportsDashboard({ report, dateFrom, dateTo }: ReportsDashboardProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('reports');
  const conversion =
    report.quotesSent > 0 ? fmt.ratePct(report.quotesAccepted / report.quotesSent) : t('noData');
  const averageSeconds = report.averageQuoteTimeSeconds;
  const averageQuoteTime = (() => {
    if (averageSeconds === null) return t('time.noData');
    const totalMinutes = Math.round(averageSeconds / 60);
    const days = Math.floor(totalMinutes / 1440);
    const hours = Math.floor((totalMinutes % 1440) / 60);
    const minutes = totalMinutes % 60;
    const parts: string[] = [];
    if (days > 0) parts.push(t('time.days', { count: days }));
    if (hours > 0) parts.push(t('time.hours', { count: hours }));
    if (parts.length === 0 || (days === 0 && minutes > 0)) {
      parts.push(t('time.minutes', { count: minutes }));
    }
    return fmt.list(parts);
  })();
  const totalStatusCount = report.statuses.reduce((total, item) => total + item.count, 0);
  let accumulatedStatusCount = 0;

  return (
    <div className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />

      <Card className="gap-y-4">
        <CardHeader>
          <CardTitle>{t('filters.title')}</CardTitle>
          <CardDescription>{t('filters.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            action={ROUTES.reports}
            className="grid grid-cols-1 items-end gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]"
            method="get"
          >
            <label className="flex flex-col gap-y-2 text-paragraph-sm-medium">
              <span>{t('filters.from')}</span>
              <Input defaultValue={dateFrom} name="date_from" type="date" />
            </label>
            <label className="flex flex-col gap-y-2 text-paragraph-sm-medium">
              <span>{t('filters.to')}</span>
              <Input defaultValue={dateTo} name="date_to" type="date" />
            </label>
            <Button type="submit" variant="outline">
              {t('filters.apply')}
            </Button>
          </form>
        </CardContent>
      </Card>

      <section
        aria-label={t('metrics.title')}
        className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5"
      >
        <MetricCard
          description={t('metrics.ordersDescription')}
          icon={ShoppingBagIcon}
          title={t('metrics.ordersReceived')}
          value={fmt.value(report.ordersReceived)}
        />
        <MetricCard
          description={t('metrics.sentDescription')}
          icon={ChartColumnIcon}
          title={t('metrics.quotesSent')}
          value={fmt.value(report.quotesSent)}
        />
        <MetricCard
          description={t('metrics.acceptedDescription')}
          icon={PackageIcon}
          title={t('metrics.quotesAccepted')}
          value={fmt.value(report.quotesAccepted)}
        />
        <MetricCard
          description={t('metrics.conversionDescription')}
          icon={ChartColumnIcon}
          title={t('metrics.conversion')}
          value={conversion}
        />
        <MetricCard
          description={t('metrics.quoteTimeDescription')}
          icon={Clock3Icon}
          title={t('metrics.quoteTime')}
          value={averageQuoteTime}
        />
      </section>

      <section className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Card className="xl:col-span-2">
          <CardHeader>
            <CardTitle>{t('statuses.title')}</CardTitle>
            <CardDescription>{t('statuses.description')}</CardDescription>
          </CardHeader>
          <CardContent>
            {totalStatusCount > 0 ? (
              <div className="mx-auto flex w-full max-w-2xl flex-col items-center gap-x-12 gap-y-8 sm:flex-row sm:justify-center">
                <div className="relative size-48 shrink-0">
                  <svg aria-hidden="true" className="size-full -rotate-90" viewBox="0 0 100 100">
                    {report.statuses.map(({ status, count }) => {
                      const segmentLength = (count / totalStatusCount) * STATUS_CHART_CIRCUMFERENCE;
                      const strokeDasharray = `${segmentLength} ${STATUS_CHART_CIRCUMFERENCE - segmentLength}`;
                      const strokeDashoffset = -accumulatedStatusCount;
                      accumulatedStatusCount += segmentLength;

                      return (
                        <circle
                          className={STATUS_CHART_COLOUR[status]?.stroke ?? 'text-primary'}
                          cx="50"
                          cy="50"
                          fill="none"
                          key={status}
                          r={STATUS_CHART_RADIUS}
                          stroke="currentColor"
                          strokeDasharray={strokeDasharray}
                          strokeDashoffset={strokeDashoffset}
                          strokeWidth="16"
                        />
                      );
                    })}
                  </svg>
                  <div className="absolute inset-0 flex flex-col items-center justify-center">
                    <span className="text-paragraph-sm text-foreground-muted">
                      {t('statuses.total')}
                    </span>
                    <span className="text-heading-4 tabular-nums">
                      {fmt.value(totalStatusCount)}
                    </span>
                  </div>
                </div>
                <ul
                  aria-label={t('statuses.title')}
                  className="flex w-full flex-col gap-y-3 sm:w-auto"
                >
                  {report.statuses.map(({ status, count }) => (
                    <li className="flex items-center justify-between gap-x-8" key={status}>
                      <span className="flex items-center gap-x-3">
                        <span
                          aria-hidden="true"
                          className={`size-3 shrink-0 rounded-full ${STATUS_CHART_COLOUR[status]?.marker ?? 'bg-primary'}`}
                        />
                        <span className="text-paragraph-sm">{t(`status.${status}`)}</span>
                      </span>
                      <span className="text-paragraph-sm-medium tabular-nums">
                        {fmt.value(count)}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : (
              <EmptyState
                description={t('empty.description')}
                icon={ChartColumnIcon}
                title={t('empty.title')}
              />
            )}
          </CardContent>
        </Card>

        <RankingCard
          description={t('materials.description')}
          emptyDescription={t('empty.description')}
          emptyTitle={t('empty.title')}
          items={report.topMaterials}
          title={t('materials.title')}
          orderLabel={(count) => t('rankings.orders', { count })}
        />
        <RankingCard
          description={t('clients.description')}
          emptyDescription={t('empty.description')}
          emptyTitle={t('empty.title')}
          items={report.topClients}
          title={t('clients.title')}
          orderLabel={(count) => t('rankings.orders', { count })}
        />
      </section>
    </div>
  );
}

interface RankingCardProps {
  title: string;
  description: string;
  emptyTitle: string;
  emptyDescription: string;
  orderLabel: (count: number) => string;
  items: { name: string; orderCount: number }[];
}

function RankingCard({
  title,
  description,
  emptyTitle,
  emptyDescription,
  orderLabel,
  items,
}: RankingCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {items.length > 0 ? (
          <ol className="flex flex-col gap-y-1">
            {items.map((item, index) => (
              <li
                className="flex items-center justify-between gap-x-4 rounded-lg px-3 py-3 even:bg-sunken"
                key={`${item.name}-${index}`}
              >
                <span className="flex min-w-0 items-center gap-x-3">
                  <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent text-paragraph-xs-semibold text-accent-foreground tabular-nums">
                    {index + 1}
                  </span>
                  <span className="truncate text-paragraph-sm">{item.name}</span>
                </span>
                <span className="shrink-0 text-paragraph-sm-medium tabular-nums">
                  {orderLabel(item.orderCount)}
                </span>
              </li>
            ))}
          </ol>
        ) : (
          <EmptyState description={emptyDescription} icon={ShoppingBagIcon} title={emptyTitle} />
        )}
      </CardContent>
    </Card>
  );
}
