'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { Badge, MetaList } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { FollowupBadge, followupDue } from '@/app/(protected)/rfqs/_components/followup-badge';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { ROUTES } from '@/config/routes';
import { formatRfqReference, type RfqRecord } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

interface RfqQueueCardProps {
  rfq: RfqRecord;
  active: boolean;
  // Set while the card tops a folded stack: how many orders the stack holds. A press then unfolds it.
  stackCount?: number;
  onExpand: () => void;
}

/*
 * One order in the queue column: enough to pick the next one without leaving the one in hand. The
 * group heading already names the status, so the card only carries the ingestion spinner, which is
 * live news rather than a repeat of it.
 */
export function RfqQueueCard({ rfq, active, stackCount, onExpand }: RfqQueueCardProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');

  const reference = formatRfqReference(rfq.quoteNumber) ?? t('list.numberPending');
  const stacked = stackCount !== undefined;

  return (
    /*
     * The selected order is a brand-tinted fill with the reference in the accent ink, so selection
     * differs from a hovered card by hue as well as by weight; hover alone is neutral.
     */
    <button
      type="button"
      aria-current={active ? 'page' : undefined}
      onClick={stacked ? onExpand : () => router.push(ROUTES.rfqsDetail(rfq.id))}
      className={cn(
        'flex size-full flex-col px-3 py-2.5 gap-y-1.5 border rounded-lg shadow-e1 text-left outline-none',
        'transition-[color,background-color,border-color,box-shadow] duration-150 ease-out-soft',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        active
          ? 'bg-accent-strong border-accent-strong active:bg-accent-stronger'
          : 'bg-background border-border hover:bg-surface-hover active:bg-surface-active',
      )}
    >
      {/* MetaList, not a hand-typed dash: a counter order has no client to put after it. The client
          is wrapped only when there is one, or the separator would survive an empty wrapper. */}
      <MetaList
        singleLine
        className={cn(
          'text-paragraph-sm-medium',
          active ? 'text-accent-foreground' : 'text-foreground',
        )}
        items={[
          reference,
          rfq.client ? (
            <span key="client" title={rfq.client}>
              {rfq.client}
            </span>
          ) : null,
        ]}
      />
      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {rfq.processing || rfq.status === 'RECEIVED' ? (
          <RfqStatusBadge status={rfq.status} processing={rfq.processing} size="sm" />
        ) : null}
        {followupDue(rfq.needsFollowup, rfq.status, Boolean(rfq.archived)) ? (
          <FollowupBadge flaggedAt={rfq.followupFlaggedAt} size="sm" />
        ) : null}
        {rfq.reviewCount > 0 ? (
          <Badge tone="warning" size="sm">
            {t('list.toReview', { count: rfq.reviewCount })}
          </Badge>
        ) : null}
        <span className="whitespace-nowrap text-paragraph-mini text-foreground-muted tabular-nums">
          {fmt.dateNumeric(rfq.createdAt)}
        </span>
      </span>
      {stacked ? (
        <span className="sr-only">{t('list.groups.stackHint', { count: stackCount })}</span>
      ) : null}
    </button>
  );
}
