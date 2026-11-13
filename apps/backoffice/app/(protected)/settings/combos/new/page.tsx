import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ComboForm } from '@/app/(protected)/settings/combos/_components/combo-form';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('comboNew');

export default async function NewComboPage() {
  await requireAdmin();
  const t = await getTranslations('combos');
  const branches = await getBranches();

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('new.title')} description={t('new.description')} />
      <ComboForm
        initialValues={{
          name: '',
          description: '',
          isActive: true,
          items: [
            { productId: '', label: '', quantity: '1' },
            { productId: '', label: '', quantity: '1' },
          ],
          // Sold everywhere unless the admin narrows it.
          branchIds: branches.map((branch) => branch.id),
        }}
        branches={branches}
      />
    </main>
  );
}
