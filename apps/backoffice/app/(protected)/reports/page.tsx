import {
  ChartColumnIcon,
  ClockIcon,
  InboxIcon,
  SendIcon,
  TrendingUpIcon,
  WalletIcon,
} from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import {
  ConfidenceBar,
  HorizontalBars,
  TimelineChart,
} from '@/app/(protected)/reports/_components/report-charts';
import { ReportFilters } from '@/app/(protected)/reports/_components/report-filters';
import { SummaryCard } from '@/app/(protected)/reports/_components/report-summary';
import { ApiError } from '@/lib/api/errors';
import { getSellerReport, type SellerReport } from '@/lib/api/reports';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('reports');

function single(value: string | string[] | undefined): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

export default async function ReportsPage({
  searchParams,
}: {
  searchParams: Promise<{ from?: string | string[]; to?: string | string[] }>;
}) {
  const params = await searchParams;
  const from = single(params.from);
  const to = single(params.to);
  const fmt = await getFormatters();
  const t = await getTranslations('reports');

  let report: SellerReport | null = null;
  try {
    report = await getSellerReport(from, to);
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 400) throw error;
  }

  if (!report) {
    return (
      <PageShell>
        <PageHeader title={t('title')} description={t('description')} />
        <Card className="gap-y-0 py-5">
          <CardContent>
            <ReportFilters from={from ?? ''} to={to ?? ''} />
            <p role="alert" className="mt-3 text-paragraph-xs text-danger-foreground">
              {t('filters.invalidPeriod')}
            </p>
          </CardContent>
        </Card>
      </PageShell>
    );
  }

  const tStatus = await getTranslations('rfqs.status');

  const formatDuration = (seconds: number | null) => {
    if (seconds === null) return t('noDataShort');
    if (seconds < 60) return t('duration.seconds', { count: Math.round(seconds) });
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return t('duration.minutes', { count: minutes });
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    return rest === 0
      ? t('duration.hours', { count: hours })
      : t('duration.hoursMinutes', { hours, minutes: rest });
  };

  const { kpis, aiMatch } = report;
  const noActivity =
    kpis.rfqsReceived === 0 &&
    kpis.quotesSent === 0 &&
    kpis.quotesAccepted === 0 &&
    report.quotesByStatus.length === 0 &&
    report.topProducts.length === 0 &&
    report.topClients.length === 0 &&
    aiMatch.total === 0;
  const responseDetail =
    kpis.responseTime.sample > 0 && kpis.responseTime.medianSeconds !== null
      ? t('responseDetail', {
          median: formatDuration(kpis.responseTime.medianSeconds),
          count: kpis.responseTime.sample,
        })
      : t('noDataShort');

  return (
    <PageShell>
      <PageHeader
        title={t('title')}
        description={t(report.scope.multiBranch ? 'descriptionAll' : 'description')}
      />
      <Card className="gap-y-0 py-5">
        <CardContent>
          <ReportFilters
            key={`${report.range.from}-${report.range.to}`}
            from={report.range.from}
            to={report.range.to}
          />
        </CardContent>
      </Card>

      {noActivity ? (
        <EmptyState
          icon={ChartColumnIcon}
          size="lg"
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <SummaryCard
              icon={InboxIcon}
              label={t('kpis.rfqs')}
              value={fmt.value(kpis.rfqsReceived)}
            />
            <SummaryCard
              icon={SendIcon}
              label={t('kpis.sent')}
              value={fmt.value(kpis.quotesSent)}
            />
            <SummaryCard
              icon={ChartColumnIcon}
              label={t('kpis.accepted')}
              value={fmt.value(kpis.quotesAccepted)}
              detail={t('conversionDetail', {
                accepted: kpis.quotesAccepted,
                sent: kpis.quotesSent,
              })}
            />
            <SummaryCard
              icon={TrendingUpIcon}
              label={t('kpis.conversion')}
              value={
                kpis.conversionRate === null ? t('noDataShort') : fmt.ratePct(kpis.conversionRate)
              }
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
          </div>

          <div className="grid gap-5 xl:grid-cols-2">
            <Card className="gap-y-4 py-5">
              <CardHeader>
                <CardTitle>{t('status.title')}</CardTitle>
              </CardHeader>
              <CardContent>
                {report.quotesByStatus.length === 0 ? (
                  <EmptyState icon={ChartColumnIcon} size="inline" title={t('status.empty')} />
                ) : (
                  <HorizontalBars
                    items={report.quotesByStatus.map((item) => ({
                      label: tStatus(item.status),
                      value: item.count,
                      tone:
                        item.status === 'ACCEPTED'
                          ? 'accepted'
                          : item.status === 'SENT'
                            ? 'sent'
                            : 'neutral',
                    }))}
                  />
                )}
              </CardContent>
            </Card>

            <Card className="gap-y-4 py-5">
              <CardHeader>
                <CardTitle>{t('timeline.title')}</CardTitle>
              </CardHeader>
              <CardContent>
                <TimelineChart
                  points={report.timeline}
                  sentLabel={t('timeline.sent')}
                  acceptedLabel={t('timeline.accepted')}
                  dateLabel={t('timeline.date')}
                  ariaLabel={t('timeline.aria', {
                    from: report.range.from,
                    to: report.range.to,
                  })}
                />
              </CardContent>
            </Card>

            <Card className="gap-y-0 overflow-hidden py-0">
              <CardHeader className="pb-4 pt-5">
                <CardTitle>{t('products.title')}</CardTitle>
              </CardHeader>
              {report.topProducts.length === 0 ? (
                <CardContent className="pb-5">
                  <EmptyState icon={ChartColumnIcon} size="inline" title={t('products.empty')} />
                </CardContent>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead kind="text">{t('products.name')}</TableHead>
                      <TableHead kind="count">{t('products.requests')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {report.topProducts.map((product, index) => (
                      <TableRow key={`${product.productId ?? product.name}-${index}`}>
                        <TableCell>{product.name || t('products.unnamed')}</TableCell>
                        <TableCell>{fmt.value(product.requests)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </Card>

            <Card className="gap-y-0 overflow-hidden py-0">
              <CardHeader className="pb-4 pt-5">
                <CardTitle>{t('clients.title')}</CardTitle>
              </CardHeader>
              {report.topClients.length === 0 ? (
                <CardContent className="pb-5">
                  <EmptyState icon={ChartColumnIcon} size="inline" title={t('clients.empty')} />
                </CardContent>
              ) : (
                <Table figures="end">
                  <TableHeader>
                    <TableRow>
                      <TableHead kind="text">{t('clients.name')}</TableHead>
                      <TableHead kind="money">{t('clients.acceptedAmount')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {report.topClients.map((client, index) => (
                      <TableRow key={`${client.clientId ?? client.name}-${index}`}>
                        <TableCell>{client.name || t('clients.unnamed')}</TableCell>
                        <TableCell>{fmt.currency(client.acceptedAmount)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </Card>

            <Card className="gap-y-4 py-5">
              <CardHeader>
                <CardTitle>{t('ai.title')}</CardTitle>
              </CardHeader>
              <CardContent>
                {aiMatch.total === 0 ? (
                  <EmptyState icon={ChartColumnIcon} size="inline" title={t('ai.empty')} />
                ) : (
                  <div className="flex flex-col gap-y-4">
                    <div className="flex items-end justify-between gap-x-4">
                      <div>
                        <p className="text-heading-4 tabular-nums text-foreground">
                          {aiMatch.rate === null ? t('noDataShort') : fmt.ratePct(aiMatch.rate)}
                        </p>
                        <p className="text-paragraph-xs text-foreground-muted">
                          {t('ai.matched', { matched: aiMatch.matched, total: aiMatch.total })}
                        </p>
                      </div>
                    </div>
                    <ConfidenceBar
                      low={aiMatch.confidence.low}
                      medium={aiMatch.confidence.medium}
                      high={aiMatch.confidence.high}
                      labels={{
                        low: t('ai.low'),
                        medium: t('ai.medium'),
                        high: t('ai.high'),
                      }}
                    />
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {report.scope.multiBranch ? (
            <Card className="gap-y-0 overflow-hidden py-0">
              <CardHeader className="pb-4 pt-5">
                <CardTitle>{t('branches.title')}</CardTitle>
              </CardHeader>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead kind="text">{t('branches.name')}</TableHead>
                    <TableHead kind="count">{t('branches.rfqs')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {report.branches.map((branch) => (
                    <TableRow key={branch.branchId}>
                      <TableCell>{branch.branchName}</TableCell>
                      <TableCell>{fmt.value(branch.rfqs)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          ) : null}
        </>
      )}
    </PageShell>
  );
}
