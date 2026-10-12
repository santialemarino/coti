import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button, Separator } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { InvoicingSettingsForm } from '@/app/(protected)/settings/invoicing/_components/invoicing-settings-form';
import { ROUTES } from '@/config/routes';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('invoicingSettings');

export default async function InvoicingSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('invoicing.settings');
  const settings = await getInvoicingSettings();

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <InvoicingSettingsForm settings={settings} />
      <Separator />
      <Button asChild variant="outline" className="self-start">
        <Link href={ROUTES.arcaSettings}>{t('connection')}</Link>
      </Button>
    </main>
  );
}
