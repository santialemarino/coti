import { getTranslations } from 'next-intl/server';

import { Callout } from '@repo/ui/components';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ReportsDashboard } from '@/app/(protected)/reports/_components/reports-dashboard';
import { getSellerReport } from '@/lib/api/reports';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('reports');

interface ReportsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

function isDateOnly(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00.000Z`);
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
}

export default async function ReportsPage({ searchParams }: ReportsPageProps) {
  const params = await searchParams;
  const dateFrom = typeof params.date_from === 'string' ? params.date_from : undefined;
  const dateTo = typeof params.date_to === 'string' ? params.date_to : undefined;
  const t = await getTranslations('reports');
  const invalidDates =
    (params.date_from !== undefined && dateFrom === undefined) ||
    (params.date_to !== undefined && dateTo === undefined) ||
    (dateFrom !== undefined && !isDateOnly(dateFrom)) ||
    (dateTo !== undefined && !isDateOnly(dateTo)) ||
    (dateFrom !== undefined && dateTo !== undefined && dateFrom > dateTo);

  return (
    <PageShell>
      {invalidDates ? (
        <Callout tone="danger" title={t('invalidDate.title')}>
          {t('invalidDate.description')}
        </Callout>
      ) : (
        <ReportsDashboard
          dateFrom={dateFrom}
          dateTo={dateTo}
          report={await getSellerReport({ dateFrom, dateTo })}
        />
      )}
    </PageShell>
  );
}
