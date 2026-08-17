import { getTranslations } from 'next-intl/server';

import { Brand } from '@/components/brand';

/*
 * A full-viewport screen with the lockup above a single column — every page the app shows before,
 * outside or instead of the signed-in shell: the auth flows and the 404. Shared so those cannot
 * drift apart in width, spacing or where the mark sits.
 *
 * The entrance is here rather than on each card, so it plays once per screen for the brand and the
 * content together and never a second time when a flow swaps its own stage.
 */
export async function BrandedScreen({ children }: { children: React.ReactNode }) {
  const t = await getTranslations('common');

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={t('appName')} />
        {children}
      </div>
    </main>
  );
}
