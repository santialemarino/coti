import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCASetupGuide } from '@/app/(protected)/settings/invoicing/arca/_components/arca-setup-guide';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('arcaSettings');

export default async function ARCASettingsPage() {
  await requireAdmin();
  const t = await getTranslations('arca');
  const [setup, settings] = await Promise.all([getARCASetup(), getInvoicingSettings()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <ARCASetupGuide
        setup={setup}
        accountTaxId={settings.taxId}
        branches={settings.branches}
        now={new Date().toISOString()}
      />
    </main>
  );
}
