import { getTranslations } from 'next-intl/server';

import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import { apiRequest } from '@/lib/api/client';
import type { RfqChannel, RfqListItem, RfqRecord, RfqStatus } from '@/lib/api/rfqs';
import { getActiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('rfqs');

function mapListItem(item: RfqListItem): RfqRecord {
  return {
    id: item.id,
    client: item.client ?? '',
    createdAt: item.created_at,
    channel: item.channel as RfqChannel,
    seller: item.seller,
    branch: item.branch,
    itemCount: item.item_count,
    total: item.total ?? undefined,
    priority: 'normal',
    status: item.status.toUpperCase() as RfqStatus,
    archived: item.archived_at != null,
  };
}

async function fetchRfqs(): Promise<RfqRecord[]> {
  const items = await apiRequest<RfqListItem[]>({ path: '/v1/rfqs' });
  return (items ?? []).map(mapListItem);
}

export default async function RfqsPage() {
  const t = await getTranslations('rfqs');
  const session = await getSession();
  const activeBranchId = await getActiveBranchId();
  const initialRecords = await fetchRfqs();

  return (
    <div className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{session ? t('greeting', { name: session.name }) : ''}</h1>
        <p className="text-paragraph text-foreground-muted">{t('selectHint')}</p>
      </header>
      <RfqDashboard initialRecords={initialRecords} activeBranchId={activeBranchId ?? null} />
    </div>
  );
}
