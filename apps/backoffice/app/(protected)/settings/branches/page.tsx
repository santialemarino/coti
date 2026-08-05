import { getTranslations } from 'next-intl/server';

import { BranchTable } from '@/app/(protected)/settings/branches/_components/branch-table';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('branchSettings');

export default async function BranchSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('branches');
  // Every active branch of the account, because the reader is admin-aware. A closed one is not
  // listed at all, which is why this screen closes branches but cannot reopen one.
  const branches = await getBranches();

  return (
    <main className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">{t('description')}</p>
      </header>
      <BranchTable branches={branches} />
    </main>
  );
}
