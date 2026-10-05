'use client';

import { ClipboardListIcon, LinkIcon, MailIcon, MessageCircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { MetaList } from '@repo/ui/components';
import { FollowupBadge, followupDue } from '@/app/(protected)/rfqs/_components/followup-badge';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { BackLink } from '@/components/back-link';
import { ROUTES } from '@/config/routes';
import type { RfqChannel, RfqDetailResponse } from '@/lib/api/rfqs';
import { formatRfqReference, normalizeRfqStatus } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

const CHANNEL_ICON: Record<RfqChannel, typeof MailIcon> = {
  whatsapp: MessageCircleIcon,
  email: MailIcon,
  webapp: LinkIcon,
  manual_entry: ClipboardListIcon,
};

interface RfqDetailHeaderProps {
  detail: RfqDetailResponse;
}

export function RfqDetailHeader({ detail }: RfqDetailHeaderProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const { rfq } = detail;

  const channel = rfq.channel as RfqChannel;
  const ChannelIcon = CHANNEL_ICON[channel] ?? ClipboardListIcon;

  return (
    <div className="flex flex-col gap-y-2">
      <BackLink href={ROUTES.home} label={t('detail.backToList')} />

      <div className="flex items-center justify-between gap-x-4">
        <h2 className="min-w-0 truncate text-heading-3 text-foreground max-lg:text-heading-2">
          {formatRfqReference(rfq.quote_number) ?? t('list.numberPending')}
        </h2>
        <div className="flex shrink-0 items-center gap-x-2">
          {followupDue(
            rfq.needs_followup,
            normalizeRfqStatus(rfq.status),
            rfq.archived_at != null,
          ) ? (
            <FollowupBadge flaggedAt={rfq.followup_flagged_at} />
          ) : null}
          <RfqStatusBadge
            status={normalizeRfqStatus(rfq.status)}
            archived={rfq.archived_at != null}
          />
        </div>
      </div>

      <MetaList
        items={[
          <span key="date" className="text-paragraph-sm-medium text-foreground">
            {fmt.date(rfq.created_at)}
          </span>,
          <span key="channel" className="inline-flex items-center gap-x-1">
            <ChannelIcon aria-hidden="true" className="size-3.5" />
            {t(`channels.${channel}`)}
          </span>,
          rfq.seller || t('list.unassigned'),
          rfq.branch,
        ]}
      />
    </div>
  );
}
