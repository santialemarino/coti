'use client';

import Link from 'next/link';
import {
  ArrowLeftIcon,
  ClipboardListIcon,
  LinkIcon,
  MailIcon,
  MessageCircleIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import { MetaList } from '@repo/ui/components';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
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
      {/*
       * The way back is its own line with its own label. Sitting it beside the heading meant
       * centring a 16px glyph against a 30px line, which never lands anywhere that looks
       * deliberate — and left the only exit from the screen unnamed.
       */}
      <Link
        href={ROUTES.rfqs}
        className="group/back flex w-fit items-center gap-x-1.5 rounded-sm outline-none text-paragraph-xs-medium text-foreground-muted transition-colors duration-200 ease-out-soft hover:text-foreground focus-visible:text-foreground"
      >
        <ArrowLeftIcon
          aria-hidden="true"
          className="size-3.5 group-focus-visible/back:animate-focus-bump-soft"
        />
        {t('detail.backToList')}
      </Link>

      <div className="flex items-center justify-between gap-x-4">
        <h2 className="min-w-0 truncate text-heading-3 text-foreground">
          {formatRfqReference(rfq.quote_number) ?? t('list.numberPending')}
        </h2>
        <RfqStatusBadge
          status={normalizeRfqStatus(rfq.status)}
          archived={rfq.archived_at != null}
        />
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
