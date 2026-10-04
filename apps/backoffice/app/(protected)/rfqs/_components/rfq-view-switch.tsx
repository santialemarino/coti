'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { ToggleGroup, ToggleGroupItem } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';

export type RfqView = 'queue' | 'table';

const VIEW_ROUTE: Record<RfqView, string> = { queue: ROUTES.home, table: ROUTES.rfqs };

/*
 * The two ways to look at the orders. It sits beside the "Pedidos" title in both — the column's
 * header and the table's page header — so the way across is in the same place from either side.
 */
export function RfqViewSwitch({ view }: { view: RfqView }) {
  const router = useRouter();
  const t = useTranslations('rfqs.view');
  const other: RfqView = view === 'queue' ? 'table' : 'queue';

  useEffect(() => {
    router.prefetch(VIEW_ROUTE[other]);
  }, [other, router]);

  return (
    <ToggleGroup
      type="single"
      size="sm"
      value={view}
      aria-label={t('label')}
      // Pressing the current view would clear a single group; the view never goes unset.
      onValueChange={(next) => {
        if (next && next !== view) router.push(VIEW_ROUTE[next as RfqView]);
      }}
    >
      <ToggleGroupItem value="queue">{t('queue')}</ToggleGroupItem>
      <ToggleGroupItem value="table">{t('table')}</ToggleGroupItem>
    </ToggleGroup>
  );
}
