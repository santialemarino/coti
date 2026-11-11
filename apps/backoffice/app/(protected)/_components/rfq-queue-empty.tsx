'use client';

import Link from 'next/link';
import { InboxIcon, TableIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, EmptyState } from '@repo/ui/components';
import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { RfqCreateButton } from '@/app/(protected)/rfqs/_components/rfq-create-button';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { ROUTES } from '@/config/routes';
import type { Onboarding } from '@/lib/api/onboarding';

interface RfqQueueEmptyProps {
  /* An administrator's unfinished setup, when its card is not hidden. */
  onboarding: Onboarding | null;
}

/*
 * What the right-hand pane shows before an order is picked. It is the first screen of the day, so
 * it carries the two things a seller starts with: take one from the rail, or create a new one.
 */
export function RfqQueueEmpty({ onboarding }: RfqQueueEmptyProps) {
  const t = useTranslations('home');
  const tRfqs = useTranslations('rfqs');
  const { userName } = useRfqList();

  const actions = (
    <div className="flex flex-wrap items-center justify-center gap-3">
      <RfqCreateButton />
      <Button asChild variant="outline">
        <Link href={ROUTES.rfqs}>
          <TableIcon aria-hidden="true" />
          {t('seeAllOrders')}
        </Link>
      </Button>
    </div>
  );

  if (!onboarding)
    return (
      <div className="flex h-full flex-col items-center justify-center pb-8">
        <EmptyState
          icon={InboxIcon}
          size="lg"
          title={tRfqs('greeting', { name: userName })}
          description={t('queueHint')}
        >
          {actions}
        </EmptyState>
      </div>
    );

  // Beside the setup card the pane is no longer empty: the greeting drops its icon so both fit.
  return (
    <div className="flex min-h-full flex-col items-center px-6 pt-6 pb-8 gap-y-6">
      <section className="flex flex-col items-center gap-y-3 text-center">
        <div className="flex flex-col gap-y-1">
          <h2 className="text-heading-6">{tRfqs('greeting', { name: userName })}</h2>
          <p className="text-paragraph-sm text-foreground-muted">{t('queueHint')}</p>
        </div>
        {actions}
      </section>
      <OnboardingChecklist onboarding={onboarding} placement="home" className="w-full max-w-xl" />
    </div>
  );
}
