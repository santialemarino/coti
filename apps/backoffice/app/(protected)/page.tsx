import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('home');

export default async function HomePage() {
  const session = await getSession();
  const t = await getTranslations('home');
  const common = await getTranslations('common');

  return (
    <main className="flex flex-col px-6 py-10 gap-y-6">
      <h1 className="text-3xl font-bold">{t('title')}</h1>
      <p className="text-muted-foreground">
        {t('signedInAs', { role: session ? common(`roles.${session.role}`) : '' })}
      </p>
      <Link href={ROUTES.priceSettings} className="underline">
        {t('links.prices')}
      </Link>
    </main>
  );
}
