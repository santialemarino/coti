import { ChartColumnIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, CardContent, EmptyState } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ReportAIMatch } from '@/app/(protected)/reports/_components/report-ai-match';
import { ReportBranches } from '@/app/(protected)/reports/_components/report-branches';
import {
  RankingList,
  StatusPieChart,
  statusTone,
  TrendChart,
} from '@/app/(protected)/reports/_components/report-charts';
import { ReportDownloadButton } from '@/app/(protected)/reports/_components/report-download-button';
import { ReportFilters } from '@/app/(protected)/reports/_components/report-filters';
import { ReportGranularityToggle } from '@/app/(protected)/reports/_components/report-granularity';
import { ReportKpis } from '@/app/(protected)/reports/_components/report-kpis';
import { ReportSection } from '@/app/(protected)/reports/_components/report-section';
import { ReportTopClients } from '@/app/(protected)/reports/_components/report-top-clients';
import { getBranches } from '@/lib/api/branches';
import { getClients, getClientTags } from '@/lib/api/clients';
import { ApiError } from '@/lib/api/errors';
import {
  getSellerReport,
  type SellerReport,
  type SellerReportFilterSelection,
} from '@/lib/api/reports';
import { getSellers } from '@/lib/api/users';
import { getActiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

type SearchParam = string | string[] | undefined;
type ReportParam =
  | 'from'
  | 'to'
  | 'status'
  | 'channel'
  | 'seller_id'
  | 'client_id'
  | 'branch_id'
  | 'tag_id'
  | 'granularity';

interface FilterOption {
  id: string;
  name: string;
}

export const generateMetadata = () => generatePageMetadata('reports');

function single(value: SearchParam): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function many(value: SearchParam): string[] {
  const values = Array.isArray(value) ? value : value ? [value] : [];
  return values
    .flatMap((item) => item.split(','))
    .map((item) => item.trim())
    .filter(Boolean);
}

// A filter list that fails to load leaves its control empty; the report itself still renders.
async function clientOptions(): Promise<FilterOption[]> {
  try {
    return (await getClients())
      .filter((client) => client.name)
      .map((client) => ({ id: client.id, name: client.name as string }));
  } catch {
    return [];
  }
}

async function sellerOptions(): Promise<FilterOption[]> {
  try {
    return await getSellers();
  } catch {
    return [];
  }
}

async function tagOptions(): Promise<FilterOption[]> {
  try {
    return (await getClientTags()).map((tag) => ({ id: tag.id, name: tag.name }));
  } catch {
    return [];
  }
}

async function branchOptions(): Promise<FilterOption[]> {
  try {
    return (await getBranches()).map((branch) => ({ id: branch.id, name: branch.name }));
  } catch {
    return [];
  }
}

// A rejected selection (a bad period or filter) comes back as null so the page can say so.
async function reportFor(selection: SellerReportFilterSelection): Promise<SellerReport | null> {
  try {
    return await getSellerReport(selection);
  } catch (error) {
    if (error instanceof ApiError && error.code === 'INVALID_INPUT') return null;
    throw error;
  }
}

export default async function ReportsPage({
  searchParams,
}: {
  searchParams: Promise<Partial<Record<ReportParam, SearchParam>>>;
}) {
  const params = await searchParams;
  const t = await getTranslations('reports');
  const tStatus = await getTranslations('reports.statusLabels');
  const fmt = await getFormatters();
  const session = await getSession();
  const isAdmin = session?.role === ADMIN_ROLE;

  const selection: SellerReportFilterSelection = {
    from: single(params.from),
    to: single(params.to),
    statuses: many(params.status).map((value) => value.toUpperCase()),
    channel: single(params.channel),
    sellerId: isAdmin ? single(params.seller_id) : undefined,
    clientId: single(params.client_id),
    branchIds: many(params.branch_id),
    tagIds: many(params.tag_id),
    granularity: single(params.granularity),
  };
  const [clients, sellers, tags, branches, activeBranchId, report] = await Promise.all([
    clientOptions(),
    isAdmin ? sellerOptions() : Promise.resolve([]),
    tagOptions(),
    branchOptions(),
    getActiveBranchId(),
    reportFor(selection),
  ]);
  const showBranch = !activeBranchId && branches.length > 1;

  // The server's echo wins over the URL, so a defaulted period seeds the form too.
  const applied = report
    ? {
        from: report.range.from,
        to: report.range.to,
        statuses: report.filters.statuses,
        channel: report.filters.channel ?? undefined,
        sellerId: report.filters.sellerId ?? undefined,
        clientId: report.filters.clientId ?? undefined,
        branchIds: report.filters.branches.map((branch) => branch.id),
        tagIds: report.filters.tags.map((tag) => tag.id),
      }
    : { ...selection, from: selection.from ?? '', to: selection.to ?? '' };
  const filtersKey = [
    applied.from,
    applied.to,
    applied.statuses?.join(','),
    applied.channel,
    applied.sellerId,
    applied.clientId,
    applied.branchIds?.join(','),
    applied.tagIds?.join(','),
  ].join('|');

  const noActivity =
    report !== null &&
    report.kpis.rfqsReceived === 0 &&
    report.kpis.quotesSent === 0 &&
    report.kpis.quotesAccepted === 0 &&
    report.quotesByStatus.length === 0 &&
    report.topProducts.length === 0 &&
    report.topRevenueProducts.length === 0 &&
    report.topClients.length === 0 &&
    report.aiMatch.total === 0;

  return (
    <PageShell>
      <PageHeader
        title={t('title')}
        description={t(report?.scope.multiBranch ? 'descriptionAll' : 'description')}
        actions={
          report ? (
            <ReportDownloadButton
              from={report.range.from}
              to={report.range.to}
              statuses={report.filters.statuses}
              channel={report.filters.channel}
              sellerId={report.filters.sellerId}
              clientId={report.filters.clientId}
              branchIds={report.filters.branches.map((branch) => branch.id)}
              tagIds={report.filters.tags.map((tag) => tag.id)}
            />
          ) : undefined
        }
      />
      <Card className="gap-y-0 py-5">
        <CardContent>
          <ReportFilters
            key={filtersKey}
            from={applied.from ?? ''}
            to={applied.to ?? ''}
            statuses={applied.statuses}
            channel={applied.channel}
            sellerId={applied.sellerId}
            clientId={applied.clientId}
            branchIds={applied.branchIds}
            tagIds={applied.tagIds}
            sellers={sellers}
            clients={clients}
            branches={branches}
            tags={tags}
            showSeller={isAdmin}
            showBranch={showBranch}
            granularity={selection.granularity}
          />
          {report ? null : (
            <p role="alert" className="mt-3 text-paragraph-xs text-danger-foreground">
              {t('filters.invalid')}
            </p>
          )}
        </CardContent>
      </Card>

      {report && noActivity ? (
        <EmptyState
          icon={ChartColumnIcon}
          size="lg"
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : null}

      {report && !noActivity ? (
        <>
          <ReportKpis kpis={report.kpis} fmt={fmt} />

          <div className="grid gap-5 xl:grid-cols-12">
            <ReportSection
              title={t('status.title')}
              description={t('status.description')}
              empty={report.quotesByStatus.length === 0 ? t('status.empty') : undefined}
              className="xl:col-span-5"
            >
              <StatusPieChart
                items={report.quotesByStatus.map((item) => ({
                  key: item.status,
                  label: tStatus(item.status),
                  value: item.count,
                  tone: statusTone(item.status),
                }))}
                summary={t('status.total', {
                  count: report.quotesByStatus.reduce((total, item) => total + item.count, 0),
                })}
                ariaLabel={t('status.aria')}
                label={t('status.label')}
                valueLabel={t('status.count')}
                formatValue={fmt.value}
                formatShare={fmt.ratePct}
              />
            </ReportSection>

            <ReportSection
              title={t('timeline.title')}
              description={t(`timeline.by.${report.granularity}`)}
              action={<ReportGranularityToggle value={report.granularity} />}
              className="xl:col-span-7"
            >
              <TrendChart
                points={report.timeline}
                labels={{
                  rfqs: t('timeline.rfqs'),
                  sent: t('timeline.sent'),
                  accepted: t('timeline.accepted'),
                  conversion: t('timeline.conversion'),
                  bucket: t('timeline.bucket'),
                }}
                ariaLabel={t('timeline.aria', {
                  from: fmt.date(report.range.from),
                  to: fmt.date(report.range.to),
                })}
                scrollLabel={t('timeline.scroll')}
                formatBucket={(date) => {
                  if (report.granularity === 'month') return fmt.month(date);
                  if (report.granularity === 'week') {
                    return t('timeline.weekOf', { date: fmt.dateNumeric(date) });
                  }
                  return fmt.dateNumeric(date);
                }}
                formatValue={fmt.value}
                formatShare={fmt.ratePct}
              />
            </ReportSection>

            <ReportSection
              flush
              title={t('products.title')}
              description={t('products.description')}
              empty={report.topProducts.length === 0 ? t('products.empty') : undefined}
              className="xl:col-span-6"
            >
              <RankingList
                items={report.topProducts.map((product) => ({
                  key: product.productId ?? `unmatched-${product.name}`,
                  label: product.name || t('products.unnamed'),
                  value: t('products.requests', { count: product.requests }),
                  magnitude: product.requests,
                }))}
              />
            </ReportSection>

            <ReportSection
              flush
              title={t('revenueProducts.title')}
              description={t('revenueProducts.description')}
              empty={
                report.topRevenueProducts.length === 0 ? t('revenueProducts.empty') : undefined
              }
              className="xl:col-span-6"
            >
              <RankingList
                tone="accepted"
                items={report.topRevenueProducts.map((product) => ({
                  key: product.productId,
                  label: product.name || t('products.unnamed'),
                  value: fmt.currency(product.revenue),
                  magnitude: Number(product.revenue),
                }))}
              />
            </ReportSection>

            <ReportTopClients clients={report.topClients} fmt={fmt} className="xl:col-span-7" />
            <ReportAIMatch aiMatch={report.aiMatch} fmt={fmt} className="xl:col-span-5" />
          </div>

          {report.scope.multiBranch ? (
            <ReportBranches branches={report.branches} fmt={fmt} />
          ) : null}
        </>
      ) : null}
    </PageShell>
  );
}
