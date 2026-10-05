'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { InboxIcon, PlusIcon, TableIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, EmptyState } from '@repo/ui/components';
import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { CreateRfqDialog } from '@/app/(protected)/rfqs/_components/create-rfq-dialog';
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
  const router = useRouter();
  const t = useTranslations('home');
  const tRfqs = useTranslations('rfqs');
  const { activeBranchId, userName } = useRfqList();
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <div className="flex h-full flex-col items-center justify-center pb-8 gap-y-2">
      {/* Beside the setup card the pane is no longer empty, so the greeting takes the smaller size. */}
      <EmptyState
        icon={InboxIcon}
        size={onboarding ? 'sm' : 'lg'}
        title={tRfqs('greeting', { name: userName })}
        description={t('queueHint')}
      >
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon aria-hidden="true" />
            {tRfqs('list.create')}
          </Button>
          <Button asChild variant="outline">
            <Link href={ROUTES.rfqs}>
              <TableIcon aria-hidden="true" />
              {t('seeAllOrders')}
            </Link>
          </Button>
        </div>
      </EmptyState>
      {onboarding ? (
        <OnboardingChecklist onboarding={onboarding} placement="home" className="max-w-xl" />
      ) : null}
      <CreateRfqDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => router.refresh()}
        activeBranchId={activeBranchId}
      />
    </div>
  );
}
