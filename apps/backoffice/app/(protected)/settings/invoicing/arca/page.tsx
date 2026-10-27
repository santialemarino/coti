import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCASetupGuide } from '@/app/(protected)/settings/invoicing/arca/_components/arca-setup-guide';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';
import { ARCA_CERTIFICATE_RENEWAL_DAYS } from '@/lib/config';
import { arcaConnectionState } from '@/lib/utils/arca-connection';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('arcaSettings');

export default async function ARCASettingsPage() {
  await requireAdmin();
  // The guide is part of invoicing with Coti; off, the switch that brings it back is one page up.
  if (!(await getAccount()).invoicingEnabled) redirect(ROUTES.invoicingSettings);
  const t = await getTranslations('arca');
  const [setup, settings] = await Promise.all([getARCASetup(), getInvoicingSettings()]);
  const state = arcaConnectionState(
    setup,
    settings.branches,
    new Date().toISOString(),
    ARCA_CERTIFICATE_RENEWAL_DAYS,
  );

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <ARCASetupGuide
        setup={setup}
        accountTaxId={settings.taxId}
        branches={settings.branches}
        state={state}
      />
    </main>
  );
}
