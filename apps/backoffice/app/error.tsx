'use client';

import { useTranslations } from 'next-intl';

import { Brand } from '@/components/brand';
import { ErrorCard } from '@/components/error-card';

// Catches what a route group's own boundary cannot: a failure in one of their layouts.
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations('common');

  return (
    // The 404's frame, inline: `BrandedScreen` is a server component and a boundary is a client one.
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={t('appName')} />
        <ErrorCard reset={reset} />
      </div>
    </main>
  );
}
