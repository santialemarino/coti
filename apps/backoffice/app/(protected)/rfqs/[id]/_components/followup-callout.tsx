'use client';

import { useTranslations } from 'next-intl';

import { Callout } from '@repo/ui/components';
import { SendQuoteDialog } from '@/app/(protected)/rfqs/[id]/_components/send-quote-dialog';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

interface FollowupCalloutProps {
  quoteId: string | null;
  branchId: string;
  origin: 'customer' | 'seller';
  quietSince: string | null;
  detail?: RfqDetailResponse;
  onSent?: () => Promise<void>;
}
export function FollowupCallout({
  quoteId,
  branchId,
  origin,
  quietSince,
  detail,
  onSent,
}: FollowupCalloutProps) {
  const t = useTranslations('rfqs.detail.callouts.followup');
  const fmt = useFormatters();
  return (
    <Callout
      tone="followup"
      title={t('title')}
      action={
        origin === 'customer' && quoteId && detail && onSent ? (
          <SendQuoteDialog detail={detail} branchId={branchId} onSent={onSent} followup />
        ) : null
      }
    >
      {quietSince ? t(origin, { date: fmt.date(quietSince) }) : t(origin + 'Undated')}
    </Callout>
  );
}
