import { getTranslations } from 'next-intl/server';

import { RfqDetailSplitView } from '@/app/(protected)/rfqs/[id]/_components/rfq-detail-split-view';
import { apiRequest } from '@/lib/api/client';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('rfqs');

async function fetchRfqDetail(id: string): Promise<RfqDetailResponse | null> {
  try {
    return await apiRequest<RfqDetailResponse>({ path: `/v1/rfqs/${id}` });
  } catch (err) {
    if (err instanceof ApiError && errorCodeOf(err) === 'NOT_FOUND') {
      return null;
    }
    throw err;
  }
}

export default async function RfqDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations('rfqs');
  const detail = await fetchRfqDetail(id);

  if (!detail) {
    return (
      <div className="flex flex-1 items-stretch bg-body-background">
        <main className="min-w-0 flex-1 px-6 py-10 lg:px-10">
          <div className="flex flex-col items-center justify-center gap-y-4 py-16">
            <h2 className="text-heading-3">{t('detail.notFound')}</h2>
            <p className="text-paragraph text-foreground-muted">
              {t('detail.notFoundDescription')}
            </p>
          </div>
        </main>
      </div>
    );
  }

  return <RfqDetailSplitView detail={detail} />;
}
