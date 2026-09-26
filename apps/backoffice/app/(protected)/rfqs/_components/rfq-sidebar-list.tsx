'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { InboxIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Badge, EmptyState, MetaList } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { ROUTES } from '@/config/routes';
import { formatRfqReference, type RfqRecord } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

interface RfqSidebarListProps {
  records: RfqRecord[];
  activeRfqId: string | null;
}

/*
 * The queue rail: one line per order, enough to pick the next one without leaving the one in hand.
 * Archived orders are left out — they are archived precisely so they stop appearing here; the
 * dashboard's status filter is where they are found again.
 */
export function RfqSidebarList({ records, activeRfqId }: RfqSidebarListProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');

  const open = useMemo(() => records.filter((rfq) => !rfq.archived), [records]);

  if (open.length === 0) {
    return <EmptyState icon={InboxIcon} title={t('list.empty.title')} />;
  }

  return (
    <nav className="flex flex-col p-2" aria-label={t('list.title')}>
      <ul className="flex flex-col gap-y-0.5">
        {open.map((rfq) => {
          const isActive = rfq.id === activeRfqId;
          const reference = formatRfqReference(rfq.quoteNumber) ?? t('list.numberPending');

          return (
            <li key={rfq.id}>
              {/*
               * The selected order is an inset filled block, not a rule against the pane's edge.
               * A 2px bar at x=0 sits a whole gutter away from the row it belongs to, so it reads as
               * a border on the panel rather than as a mark on the row — and at two pixels it is the
               * quietest thing on a rail full of coloured pills. The fill is brand-tinted and the
               * reference takes the accent ink, so selection differs from a hovered row by hue as
               * well as by weight; hover alone is neutral.
               */}
              <button
                type="button"
                aria-current={isActive ? 'page' : undefined}
                onClick={() => router.push(ROUTES.rfqsDetail(rfq.id))}
                className={cn(
                  'flex w-full flex-col px-3 py-2.5 gap-y-1.5 rounded-lg text-left outline-none',
                  'transition-[color,background-color] duration-150 ease-out-soft',
                  'focus-visible:ring-3 focus-visible:ring-ring/45',
                  isActive
                    ? 'bg-accent-strong active:bg-accent-stronger'
                    : 'hover:bg-surface-hover active:bg-surface-active',
                )}
              >
                {/* MetaList, not a hand-typed dash: a counter order has no client to put after it.
                    The client is wrapped only when there is one — a wrapper around an empty string
                    is not empty, and the separator would survive it. */}
                <MetaList
                  className={cn(
                    'min-w-0 flex-nowrap text-paragraph-sm-medium',
                    isActive ? 'text-accent-foreground' : 'text-foreground',
                  )}
                  items={[
                    reference,
                    rfq.client ? (
                      <span key="client" className="truncate">
                        {rfq.client}
                      </span>
                    ) : null,
                  ]}
                />
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <RfqStatusBadge status={rfq.status} processing={rfq.processing} size="sm" />
                  {rfq.reviewCount > 0 ? (
                    <Badge tone="warning" size="sm">
                      {t('list.toReview', { count: rfq.reviewCount })}
                    </Badge>
                  ) : null}
                  <span className="text-paragraph-mini text-foreground-muted tabular-nums">
                    {fmt.dateNumeric(rfq.createdAt)}
                  </span>
                </div>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
