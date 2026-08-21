import { getTranslations } from 'next-intl/server';

import { Callout } from '@repo/ui/components';
import { CatalogImport } from '@/components/catalog-import';
import { getBranches } from '@/lib/api/branches';
import { getActiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('catalogSettings');

export default async function CatalogSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('catalogImport');
  const branches = await getBranches();
  const activeBranchId = await getActiveBranchId();
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
        <CatalogImport branch={branch} />
      ) : (
        <Callout tone="warning" title={t('noBranch.title')}>
          {t('noBranch.description')}
        </Callout>
      )}
    </main>
  );
}
