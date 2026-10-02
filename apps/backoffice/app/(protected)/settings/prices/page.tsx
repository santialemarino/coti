import { getTranslations } from 'next-intl/server';

import { Callout } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PriceImport } from '@/app/(protected)/settings/prices/_components/price-import';
import { getBranches } from '@/lib/api/branches';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('priceSettings');

export default async function PriceSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('priceImport');
  const branches = await getBranches();
  const activeBranchId = await getEffectiveBranchId(branches);
  const branch = branches.find((candidate) => candidate.id === activeBranchId);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      {branch ? (
        <PriceImport branch={branch} />
      ) : (
        <Callout tone="warning" title={t('noBranch.title')}>
          {t('noBranch.description')}
        </Callout>
      )}
    </main>
  );
}
