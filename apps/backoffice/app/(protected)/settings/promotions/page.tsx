import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PromotionTable } from '@/app/(protected)/settings/promotions/_components/promotion-table';
import { getAccountBranches } from '@/lib/api/branches';
import { getPromotions } from '@/lib/api/promotions';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('promotionSettings');

export default async function PromotionSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('promotions');
  // Closed branches included: a rule can still name one, and its row has to say which.
  const [promotions, branches] = await Promise.all([getPromotions(), getAccountBranches()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <PromotionTable promotions={promotions} branches={branches} now={new Date().toISOString()} />
    </main>
  );
}
