'use client';

import { useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { TriangleAlertIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Card, PendingButton, StatusScreen } from '@repo/ui/components';

interface ErrorCardProps {
  reset: () => void;
}

/*
 * The recoverable state for anything a screen did not catch — most often the API answering something
 * no caller expected. Next hands a client boundary only a digest in production, so the message is
 * generic on purpose; the specific one comes from whichever action mapped the error code itself.
 */
export function ErrorCard({ reset }: ErrorCardProps) {
  const router = useRouter();
  const t = useTranslations();
  const [pending, startTransition] = useTransition();

  // The refresh re-runs the server render that failed; a bare reset would re-show its cached result.
  function retry() {
    startTransition(() => {
      router.refresh();
      reset();
    });
  }

  return (
    <Card>
      <StatusScreen icon={TriangleAlertIcon} tone="danger" title={t('errors.INTERNAL')}>
        <PendingButton
          onClick={retry}
          pending={pending}
          pendingLabel={t('common.retrying')}
          size="lg"
        >
          {t('common.retry')}
        </PendingButton>
      </StatusScreen>
    </Card>
  );
}

// The same card centred in whatever frame a route group's layout already draws.
export function ErrorPanel({ reset }: ErrorCardProps) {
  return (
    <div className="flex flex-1 items-center justify-center px-4 py-16">
      <div className="flex flex-col w-full max-w-auth-card">
        <ErrorCard reset={reset} />
      </div>
    </div>
  );
}
