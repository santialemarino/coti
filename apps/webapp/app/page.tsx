import { FileTextIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { StatusScreen } from '@repo/ui/components';
import { Brand } from '@/components/brand';

export default async function HomePage() {
  const t = await getTranslations('common');
  const tHome = await getTranslations('home');

  return (
    <main className="grid min-h-dvh place-items-center px-4 py-10">
      <div className="flex w-full max-w-auth-card flex-col items-center gap-y-8">
        <Brand variant="lockup" size="xl" label={t('appName')} />
        <StatusScreen
          icon={FileTextIcon}
          tone="info"
          title={tHome('title')}
          description={tHome('description')}
        />
      </div>
    </main>
  );
}
