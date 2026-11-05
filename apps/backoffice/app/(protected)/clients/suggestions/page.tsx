import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ClientContactSuggestions } from '@/app/(protected)/clients/_components/client-contact-suggestions';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientSuggestionsPage() {
  const t = await getTranslations('clientContactSuggestions');
  return (
    <PageShell>
      <PageHeader title={t('title')} />
      <ClientContactSuggestions />
    </PageShell>
  );
}
