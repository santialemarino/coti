'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';

/*
 * The recoverable state for anything a screen did not catch — most often the API
 * answering something no caller expected. Next hands a client boundary only a digest
 * in production, so the message is generic on purpose; the specific one comes from
 * whichever action mapped the error code itself.
 */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations();

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-6 gap-y-4">
      <p className="text-muted-foreground">{t('errors.unexpected')}</p>
      <Button onClick={reset} variant="outline">
        {t('common.retry')}
      </Button>
    </main>
  );
}
