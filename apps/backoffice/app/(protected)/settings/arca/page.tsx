import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCASetupForm } from '@/app/(protected)/settings/arca/_components/arca-setup-form';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getActiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('arcaSettings');

export default async function ARCASettingsPage() {
  await requireAdmin();
  const t = await getTranslations('arca');
  const [setup, branchId] = await Promise.all([getARCASetup(), getActiveBranchId()]);
  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <ARCASetupForm key={branchId ?? 'account'} setup={setup} branchId={branchId} />
    </main>
  );
}
