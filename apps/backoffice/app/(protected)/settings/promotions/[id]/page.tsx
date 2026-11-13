import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PromotionForm } from '@/app/(protected)/settings/promotions/_components/promotion-form';
import { promotionValues } from '@/app/(protected)/settings/promotions/promotion-values';
import { getAccountBranches } from '@/lib/api/branches';
import { getProductNames, getProductTaxonomy } from '@/lib/api/products';
import { getPromotion } from '@/lib/api/promotions';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('promotionEdit');

export default async function EditPromotionPage({ params }: { params: Promise<{ id: string }> }) {
  await requireAdmin();
  const t = await getTranslations('promotions');
  const { id } = await params;
  const [promotion, families, accountBranches] = await Promise.all([
    getPromotion(id),
    getProductTaxonomy(),
    getAccountBranches(),
  ]);
  if (!promotion) notFound();

  const productNames = await getProductNames(
    promotion.conditions.flatMap((condition) => (condition.productId ? [condition.productId] : [])),
  );
  // Open branches, plus a closed one the rule still names so the scope can show it.
  const branches = accountBranches.filter(
    (branch) => branch.isActive || branch.id === promotion.branchId,
  );

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('edit.title', { name: promotion.name })} />
      <PromotionForm
        promotionId={promotion.id}
        initialValues={promotionValues(promotion, productNames)}
        families={families}
        branches={branches}
      />
    </main>
  );
}
