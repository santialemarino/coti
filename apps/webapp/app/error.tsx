'use client';

import { TriangleAlertIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { Brand } from '@/components/brand';

/*
 * The recoverable state for anything a screen did not catch. Next hands a client boundary only a
 * digest in production, so the message is generic on purpose; a failure the page can name is worded
 * where it happened.
 */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations('common');

  return (
    // The 404's frame, so an error reads as the same product as every other outcome screen.
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={t('appName')} />
        <Card>
          <StatusScreen icon={TriangleAlertIcon} tone="danger" title={t('states.error')}>
            <Button onClick={reset} size="lg">
              {t('actions.retry')}
            </Button>
          </StatusScreen>
        </Card>
      </div>
    </main>
  );
}
