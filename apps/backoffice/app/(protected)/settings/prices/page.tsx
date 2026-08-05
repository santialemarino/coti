import { getTranslations } from 'next-intl/server';

import { Callout } from '@repo/ui/components';
import { PriceImport } from '@/app/(protected)/settings/prices/_components/price-import';
import { getBranches } from '@/lib/api/branches';
import { getActiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('priceSettings');

export default async function PriceSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('priceImport');
  const branches = await getBranches();
  const activeBranchId = await getActiveBranchId();
  // A caller who reaches one branch is never shown the switcher, so that branch is the target
  // even though they never chose it.
  const branch =
    branches.find((candidate) => candidate.id === activeBranchId) ??
    (branches.length === 1 ? branches[0] : undefined);

  return (
    <main className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">{t('description')}</p>
      </header>
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
