'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Callout, PendingButton } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { formatRfqReference, type SplitOrderResponse } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

// Past this the minutes stop reading naturally, so the callout names the time instead.
const MINUTES_SHOWN = 60;
const MINUTE_MS = 60_000;

interface SplitOrderCalloutProps {
  split: SplitOrderResponse;
  createdAt: string;
  archiving: boolean;
  onArchive: () => void;
}

/*
 * One sender's conversation became two orders: the seller sees the other one, opens it, and archives
 * this one when it only repeats or completes the other.
 */
export function SplitOrderCallout({
  split,
  createdAt,
  archiving,
  onArchive,
}: SplitOrderCalloutProps) {
  const t = useTranslations('rfqs.detail.callouts.splitOrder');
  const tList = useTranslations('rfqs.list');
  const fmt = useFormatters();

  const reference = formatRfqReference(split.quote_number) ?? tList('numberPending');
  const minutes = Math.max(1, Math.round((Date.now() - Date.parse(split.created_at)) / MINUTE_MS));
  const when =
    minutes < MINUTES_SHOWN
      ? t('minutesAgo', { minutes })
      : t('at', { time: fmt.timestamp(split.created_at) });
  const earlier = Date.parse(split.created_at) < Date.parse(createdAt);

  return (
    <Callout
      tone="info"
      title={t(earlier ? 'earlier' : 'later', { when })}
      action={
        <PendingButton
          type="button"
          variant="outline"
          size="sm"
          onClick={onArchive}
          pending={archiving}
          pendingLabel={t('archiving')}
        >
          {t('archive')}
        </PendingButton>
      }
    >
      <Link
        href={ROUTES.rfqsDetail(split.rfq_id)}
        className="rounded-sm outline-none underline underline-offset-2 transition-colors duration-150 ease-out-soft hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/45"
      >
        {t('open', { reference })}
      </Link>
    </Callout>
  );
}
