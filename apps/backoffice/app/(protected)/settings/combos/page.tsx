import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ComboTable } from '@/app/(protected)/settings/combos/_components/combo-table';
import { getAccountBranches } from '@/lib/api/branches';
import { getCombos } from '@/lib/api/combos';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('comboSettings');

export default async function ComboSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('combos');
  const [combos, branches] = await Promise.all([getCombos(), getAccountBranches()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <ComboTable combos={combos} branches={branches} />
    </main>
  );
}
