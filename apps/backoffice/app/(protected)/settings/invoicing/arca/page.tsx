import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCASetupForm } from '@/app/(protected)/settings/arca/_components/arca-setup-form';
import { getARCASetup } from '@/lib/api/arca-setup';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('arcaSettings');

export default async function ARCASettingsPage() {
  await requireAdmin();
  const t = await getTranslations('arca');
  const setup = await getARCASetup();
  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <ARCASetupForm setup={setup} />
    </main>
  );
}
