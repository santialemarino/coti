import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ClientTable } from '@/app/(protected)/clients/_components/client-table';
import { getClients } from '@/lib/api/clients';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientsPage() {
  const t = await getTranslations('clients');
  const clients = await getClients();

  return (
    <PageShell>
      <PageHeader title={t('title')} description={t('description')} />
      <ClientTable clients={clients} />
    </PageShell>
  );
}
