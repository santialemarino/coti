'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';

/*
 * The recoverable state for anything a screen did not catch. Next hands a client boundary only a
 * digest in production, so the message is generic on purpose; a failure the page can name is worded
 * where it happened.
 */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations('common');

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-6 gap-y-4">
      <p className="text-muted-foreground">{t('states.error')}</p>
      <Button onClick={reset} variant="outline">
        {t('actions.retry')}
      </Button>
    </main>
  );
}
