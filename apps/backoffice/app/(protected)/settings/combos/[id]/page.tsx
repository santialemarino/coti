import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ComboForm } from '@/app/(protected)/settings/combos/_components/combo-form';
import { getAccountBranches } from '@/lib/api/branches';
import { getCombo } from '@/lib/api/combos';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('comboEdit');

export default async function EditComboPage({ params }: { params: Promise<{ id: string }> }) {
  await requireAdmin();
  const t = await getTranslations('combos');
  const { id } = await params;
  const [combo, accountBranches] = await Promise.all([getCombo(id), getAccountBranches()]);
  if (!combo) notFound();
  // Open branches, plus a closed one the combo is still sold in, so it can be taken off.
  const branches = accountBranches.filter(
    (branch) => branch.isActive || combo.branchIds.includes(branch.id),
  );

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('edit.title', { name: combo.name })} />
      <ComboForm
        comboId={combo.id}
        initialValues={{
          name: combo.name,
          description: combo.description ?? '',
          isActive: combo.isActive,
          items: combo.items.map((item) => ({
            productId: item.productId,
            label: item.productName,
            quantity: item.quantity,
          })),
          branchIds: combo.branchIds,
        }}
        branches={branches}
      />
    </main>
  );
}
