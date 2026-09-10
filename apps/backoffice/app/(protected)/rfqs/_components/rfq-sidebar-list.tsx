'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

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
 * Compact sidebar shown when a pedido is selected. Displays only the essential
 * info per row: sequence number, status badge, and date+time. Clicking a row
 * navigates to that pedido's detail without losing the sidebar context.
 */
export function RfqSidebarList({ records, activeRfqId }: RfqSidebarListProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');

  return (
    <nav className="flex flex-col" aria-label={t('list.title')}>
      {records.length === 0 ? (
        <p className="px-4 py-8 text-center text-paragraph-sm text-foreground-muted">
          {t('list.empty.title')}
        </p>
      ) : (
        <ul className="flex flex-col">
          {records.map((rfq) => {
            const isActive = rfq.id === activeRfqId;
            const reference = formatRfqReference(rfq.quoteNumber) ?? t('list.numberPending');

            return (
              <li key={rfq.id}>
                <button
                  type="button"
                  onClick={() => router.push(ROUTES.rfqsDetail(rfq.id))}
                  className={cn(
                    'flex w-full flex-col gap-y-1.5 px-3 py-2.5 text-left outline-none transition-colors',
                    'hover:bg-accent focus-visible:bg-accent',
                    isActive && 'bg-accent border-l-2 border-l-primary',
                    !isActive && 'border-l-2 border-l-transparent',
                  )}
                >
                  <span className="truncate text-paragraph-sm-medium text-foreground">
                    {reference} — {rfq.client}
                  </span>
                  <div className="flex items-center gap-x-2">
                    <RfqStatusBadge status={rfq.status} size="sm" />
                    <span className="text-paragraph-mini text-foreground-muted">
                      {fmt.timestamp(rfq.createdAt)}
                    </span>
                  </div>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </nav>
  );
}
