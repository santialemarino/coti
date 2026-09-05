'use client';

import Link from 'next/link';
import {
  ArrowLeftIcon,
  ClipboardListIcon,
  DownloadIcon,
  LinkIcon,
  MailIcon,
  MessageCircleIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { ROUTES } from '@/config/routes';
import type { RfqChannel, RfqDetailResponse } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

function formatId(id: string): string {
  if (/^\d{1,6}$/.test(id)) return id;
  return id.replace(/-/g, '').slice(0, 6).toUpperCase();
}

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
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const { rfq } = detail;

  const channel = rfq.channel as RfqChannel;
  const ChannelIcon = CHANNEL_ICON[channel] ?? ClipboardListIcon;

  return (
    <div className="flex flex-col gap-y-3">
      {/* Title row */}
      <div className="flex items-center justify-between gap-x-4">
        <div className="flex items-center gap-x-3 min-w-0">
          <Link
            href={ROUTES.rfqs}
            aria-label={t('detail.backToList')}
            className="flex size-8 shrink-0 items-center justify-center rounded-md text-foreground-muted transition-colors hover:bg-accent hover:text-foreground focus-visible:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeftIcon className="size-4" />
          </Link>
          <h2 className="min-w-0 truncate text-heading-3 text-foreground">#{formatId(rfq.id)}</h2>
        </div>
        <RfqStatusBadge status={normalizeRfqStatus(rfq.status)} />
      </div>

      {/* Metadata — single horizontal line */}
      <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-paragraph-sm text-foreground-muted">
        <span className="font-medium text-foreground">{fmt.date(rfq.created_at)}</span>
        <span aria-hidden="true" className="text-foreground-subtle">
          ·
        </span>
        <span className="inline-flex items-center gap-x-1">
          <ChannelIcon className="size-3.5" />
          {t(`channels.${channel}`)}
        </span>
        <span aria-hidden="true" className="text-foreground-subtle">
          ·
        </span>
        <span>{rfq.seller}</span>
        <span aria-hidden="true" className="text-foreground-subtle">
          ·
        </span>
        <span>{rfq.branch}</span>
      </div>

      {channel !== 'manual_entry' && (
        <div>
          <Button type="button" variant="outline" size="sm">
            <DownloadIcon className="size-4" />
            {t('detail.diff.downloadOriginal')}
          </Button>
        </div>
      )}
    </div>
  );
}
