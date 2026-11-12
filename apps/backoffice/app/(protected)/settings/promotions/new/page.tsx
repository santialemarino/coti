import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PromotionForm } from '@/app/(protected)/settings/promotions/_components/promotion-form';
import { EMPTY_PROMOTION } from '@/app/(protected)/settings/promotions/promotion-values';
import { getBranches } from '@/lib/api/branches';
import { getProductTaxonomy } from '@/lib/api/products';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('promotionNew');

export default async function NewPromotionPage() {
  await requireAdmin();
  const t = await getTranslations('promotions');
  const [families, branches] = await Promise.all([getProductTaxonomy(), getBranches()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('new.title')} description={t('new.description')} />
      <PromotionForm initialValues={EMPTY_PROMOTION} families={families} branches={branches} />
    </main>
  );
}
