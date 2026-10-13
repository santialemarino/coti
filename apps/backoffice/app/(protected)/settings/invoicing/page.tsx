import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ARCAConnectionCard } from '@/app/(protected)/settings/invoicing/_components/arca-connection-card';
import { InvoicingSettingsForm } from '@/app/(protected)/settings/invoicing/_components/invoicing-settings-form';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('invoicingSettings');

export default async function InvoicingSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('invoicing.settings');
  const [settings, setup] = await Promise.all([getInvoicingSettings(), getARCASetup()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <ARCAConnectionCard setup={setup} now={new Date().toISOString()} />
      <InvoicingSettingsForm settings={settings} />
    </main>
  );
}
