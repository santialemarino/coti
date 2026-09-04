import { getTranslations } from 'next-intl/server';

import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('home');

export default async function HomePage() {
  const session = await getSession();
  const t = await getTranslations('home');
  const common = await getTranslations('common');

  return (
    <main className="flex flex-col px-6 py-10 gap-y-6">
      <h1 className="text-heading-2">{t('title')}</h1>
      <p className="text-paragraph text-foreground-muted">
        {t('signedInAs', { role: session ? common(`roles.${session.role}`) : '' })}
      </p>
    </main>
  );
}
