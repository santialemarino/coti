'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { InboxIcon, ListIcon, PlusIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, EmptyState } from '@repo/ui/components';
import { CreateRfqDialog } from '@/app/(protected)/rfqs/_components/create-rfq-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { ROUTES } from '@/config/routes';

/*
 * What the right-hand pane shows before an order is picked. It is the first screen of the day, so
 * it carries the two things a seller starts with: take one from the rail, or create a new one.
 */
export function RfqQueueEmpty() {
  const router = useRouter();
  const t = useTranslations('home');
  const tRfqs = useTranslations('rfqs');
  const { activeBranchId, userName } = useRfqList();
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <div className="flex h-full flex-col justify-center">
      <EmptyState
        icon={InboxIcon}
        size="lg"
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
              <ListIcon aria-hidden="true" />
              {t('seeAllOrders')}
            </Link>
          </Button>
        </div>
      </EmptyState>
      <CreateRfqDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => router.refresh()}
        activeBranchId={activeBranchId}
      />
    </div>
  );
}
