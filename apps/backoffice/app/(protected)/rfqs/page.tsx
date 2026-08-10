import { getTranslations } from 'next-intl/server';

import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('rfqs');

export default async function RfqsPage() {
  const t = await getTranslations('rfqs');
  const session = await getSession();

  return (
    <div className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{session ? t('greeting', { name: session.name }) : ''}</h1>
        <p className="text-paragraph text-foreground-muted">{t('selectHint')}</p>
      </header>
      <RfqDashboard />
    </div>
  );
}
