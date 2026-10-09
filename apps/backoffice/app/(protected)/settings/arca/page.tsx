import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCASetupForm } from '@/app/(protected)/settings/arca/_components/arca-setup-form';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getBranches } from '@/lib/api/branches';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('arcaSettings');

export default async function ARCASettingsPage() {
  await requireAdmin();
  const t = await getTranslations('arca');
  const branches = await getBranches();
  const branchId = await getEffectiveBranchId(branches);
  const setup = await getARCASetup(branchId);
  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <ARCASetupForm key={branchId ?? 'account'} setup={setup} branchId={branchId} />
    </main>
  );
}
