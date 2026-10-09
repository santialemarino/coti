import { getTranslations } from 'next-intl/server';

import { Badge, Callout, Separator } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ArcaCredentialSection } from '@/app/(protected)/settings/invoicing/_components/arca-credential-section';
import { InvoicingSettingsForm } from '@/app/(protected)/settings/invoicing/_components/invoicing-settings-form';
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
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          settings.homologation ? (
            <Badge tone="warning" dot>
              {t('homologation.badge')}
            </Badge>
          ) : undefined
        }
      />
      {settings.enabled ? null : (
        <Callout tone="warning" title={t('disabled.title')}>
          {t('disabled.body')}
        </Callout>
      )}
      {settings.homologation ? (
        <Callout tone="info" title={t('homologation.title')}>
          {t('homologation.body')}
        </Callout>
      ) : null}
      <InvoicingSettingsForm settings={settings} />
      <Separator />
      <ArcaCredentialSection credential={settings.credential} />
    </main>
  );
}
