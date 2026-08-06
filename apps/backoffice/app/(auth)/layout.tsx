import { getTranslations } from 'next-intl/server';

import { Brand } from '@/components/brand';

// Middleware bounces a signed-in caller off these routes before they render, so
// this only carries the shared frame.
export default async function AuthLayout({ children }: { children: React.ReactNode }) {
  const t = await getTranslations('common');

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      {/*
        The entrance is here rather than on each card, so it plays once per screen for the brand and
        the card together and never a second time when a flow swaps its own stage.
      */}
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={t('appName')} />
        {children}
      </div>
    </main>
  );
}
