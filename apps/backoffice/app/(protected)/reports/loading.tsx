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
      <div className="grid gap-5 xl:grid-cols-12">
        <Card className="gap-y-4 py-5 xl:col-span-5">
          <div className="px-5">
            <Skeleton className="h-5 w-48" />
          </div>
          <div className="flex items-center gap-6 px-5">
            <Skeleton className="size-40 shrink-0 rounded-full sm:size-44" />
            <div className="flex w-full flex-col gap-y-3">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-4/5" />
              <Skeleton className="h-3 w-3/5" />
              <Skeleton className="h-3 w-4/5" />
            </div>
          </div>
        </Card>
        <Card className="gap-y-4 py-5 xl:col-span-7">
          <div className="px-5">
            <Skeleton className="h-5 w-56" />
          </div>
          <div className="px-5">
            <Skeleton className="h-44 w-full" />
          </div>
        </Card>
        {Array.from({ length: 2 }, (_, index) => (
          <Card key={index} className="gap-y-4 py-5 xl:col-span-6">
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
        <Card className="gap-y-4 py-5 xl:col-span-7">
          <div className="px-5">
            <Skeleton className="h-5 w-52" />
          </div>
          <div className="flex flex-col gap-y-3 px-5">
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-4/5" />
            <Skeleton className="h-3 w-3/5" />
          </div>
        </Card>
        <Card className="gap-y-4 py-5 xl:col-span-5">
          <div className="px-5">
            <Skeleton className="h-5 w-48" />
          </div>
          <div className="flex flex-col gap-y-3 px-5">
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-4/5" />
            <Skeleton className="h-3 w-3/5" />
          </div>
        </Card>
      </div>
    </PageShell>
  );
}
