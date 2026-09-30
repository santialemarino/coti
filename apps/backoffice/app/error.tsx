'use client';

import { TriangleAlertIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { Brand } from '@/components/brand';

/*
 * The recoverable state for anything a screen did not catch — most often the API
 * answering something no caller expected. Next hands a client boundary only a digest
 * in production, so the message is generic on purpose; the specific one comes from
 * whichever action mapped the error code itself.
 */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations();

  return (
    // The 404's frame, inline: `BrandedScreen` is a server component and a boundary is a client one.
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={t('common.appName')} />
        <Card>
          <StatusScreen icon={TriangleAlertIcon} tone="danger" title={t('errors.INTERNAL')}>
            <Button onClick={reset} size="lg">
              {t('common.retry')}
            </Button>
          </StatusScreen>
        </Card>
      </div>
    </main>
  );
}
