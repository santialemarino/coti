'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { InboxIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { EmptyState, MetaList } from '@repo/ui/components';
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
    <nav className="flex flex-col" aria-label={t('list.title')}>
      <ul className="flex flex-col">
        {open.map((rfq) => {
          const isActive = rfq.id === activeRfqId;
          const reference = formatRfqReference(rfq.quoteNumber) ?? t('list.numberPending');

          return (
            <li key={rfq.id}>
              <button
                type="button"
                aria-current={isActive ? 'true' : undefined}
                onClick={() => router.push(ROUTES.rfqsDetail(rfq.id))}
                className={cn(
                  'flex w-full flex-col px-3 py-2.5 gap-y-1.5 border-l-2 text-left outline-none',
                  'transition-[color,background-color,border-color] duration-150 ease-out-soft',
                  isActive
                    ? 'border-l-primary bg-accent'
                    : 'border-l-transparent hover:bg-surface-hover active:bg-surface-active focus-visible:bg-surface-hover',
                )}
              >
                {/* MetaList, not a hand-typed dash: a counter order has no client to put after it.
                    The client is wrapped only when there is one — a wrapper around an empty string
                    is not empty, and the separator would survive it. */}
                <MetaList
                  className="min-w-0 flex-nowrap text-paragraph-sm-medium text-foreground"
                  items={[
                    reference,
                    rfq.client ? (
                      <span key="client" className="truncate">
                        {rfq.client}
                      </span>
                    ) : null,
                  ]}
                />
                <div className="flex items-center gap-x-2">
                  <RfqStatusBadge status={rfq.status} processing={rfq.processing} size="sm" />
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
