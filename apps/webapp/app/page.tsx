import { getTranslations } from 'next-intl/server';

import { Brand } from '@/components/brand';

export default async function HomePage() {
  const t = await getTranslations('common');

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-6 gap-y-6">
      <Brand variant="lockup" size="xl" label={t('appName')} />
      <p className="text-paragraph text-foreground-muted">{t('states.loading')}</p>
    </main>
  );
}
