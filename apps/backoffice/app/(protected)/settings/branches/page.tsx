import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { BranchTable } from '@/app/(protected)/settings/branches/_components/branch-table';
import { getAccountBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('branchSettings');

export default async function BranchSettingsPage({
  searchParams,
}: {
  searchParams: Promise<{ gmail?: string }>;
}) {
  await requireAdmin();
  const t = await getTranslations('branches');
  // Closed branches included, which is what lets one be reopened. The switcher reads the other
  // list, so it can never offer a branch the API would refuse.
  const branches = await getAccountBranches();
  const status = (await searchParams).gmail;
  const gmailStatus = status === 'connected' || status === 'failed' ? status : undefined;

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <BranchTable branches={branches} gmailStatus={gmailStatus} />
    </main>
  );
}
