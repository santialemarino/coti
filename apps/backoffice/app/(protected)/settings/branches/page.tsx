import { getTranslations } from 'next-intl/server';

import { BranchTable } from '@/app/(protected)/settings/branches/_components/branch-table';
import { getAccountBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('branchSettings');

export default async function BranchSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('branches');
  // Closed branches included, which is what lets one be reopened. The switcher reads the other
  // list, so it can never offer a branch the API would refuse.
  const branches = await getAccountBranches();

  return (
    <main className="flex flex-col gap-y-8">
      <h1 className="text-heading-2">{t('title')}</h1>
      <BranchTable branches={branches} />
    </main>
  );
}
