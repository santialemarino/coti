import { getTranslations } from 'next-intl/server';

import { Card, Skeleton } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';

export default async function ReportsLoading() {
  const t = await getTranslations('reports');
  return (
    <PageShell>
      <PageHeader title={t('title')} description={t('description')} />
      <Card className="gap-y-0 py-5">
        <div className="flex flex-wrap gap-4 px-5">
          <Skeleton className="h-9 w-44" />
          <Skeleton className="h-9 w-44" />
          <Skeleton className="h-9 w-24" />
        </div>
      </Card>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }, (_, index) => (
          <Card key={index} className="gap-y-3 py-5">
            <div className="px-5">
              <Skeleton className="h-4 w-36" />
            </div>
            <div className="px-5">
              <Skeleton className="h-8 w-24" />
            </div>
          </Card>
        ))}
      </div>
      <div className="grid gap-5 xl:grid-cols-2">
        {Array.from({ length: 4 }, (_, index) => (
          <Card key={index} className="gap-y-4 py-5">
            <div className="px-5">
              <Skeleton className="h-5 w-48" />
            </div>
            <div className="flex flex-col gap-y-3 px-5">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-4/5" />
              <Skeleton className="h-3 w-3/5" />
            </div>
          </Card>
        ))}
      </div>
    </PageShell>
  );
}
