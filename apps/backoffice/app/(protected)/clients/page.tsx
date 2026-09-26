import { getTranslations } from 'next-intl/server';

import { ClientTable } from '@/app/(protected)/clients/_components/client-table';
import { getClients } from '@/lib/api/clients';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientsPage() {
  const t = await getTranslations('clients');
  const clients = await getClients();

  return (
    <main className="flex flex-col gap-y-8">
      <div className="flex flex-col gap-y-1">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">{t('description')}</p>
      </div>
      <ClientTable clients={clients} />
    </main>
  );
}
