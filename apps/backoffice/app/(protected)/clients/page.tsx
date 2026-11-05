import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ClientTable } from '@/app/(protected)/clients/_components/client-table';
import { ROUTES } from '@/config/routes';
import { getClients } from '@/lib/api/clients';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientsPage() {
  const t = await getTranslations('clients');
  const clients = await getClients();

  return (
    <PageShell>
      <PageHeader title={t('title')} description={t('description')} />
      <Button variant="outline" className="self-start" asChild>
        <Link href={ROUTES.clientSuggestions}>{t('contactSuggestions')}</Link>
      </Button>
      <ClientTable clients={clients} />
    </PageShell>
  );
}
