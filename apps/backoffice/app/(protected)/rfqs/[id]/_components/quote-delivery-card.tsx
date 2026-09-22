'use client';

import { useState } from 'react';
import { CheckIcon, CopyIcon, ExternalLinkIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@repo/ui/components';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { formatRfqReference, normalizeRfqStatus } from '@/lib/api/rfqs';
import { SendQuoteDialog } from './send-quote-dialog';

// The public endpoint only resolves tokens in these states, so any other state would hand the seller
// a link the customer cannot open.
const LIVE_TRACKING_STATUSES = new Set(['SENT', 'DELIVERED', 'VIEWED']);

interface QuoteDeliveryCardProps {
  detail: RfqDetailResponse;
  onSent: () => Promise<void>;
}

/*
 * The quotation itself, as a thing the seller can act on: its reference, its version and status,
 * and the link the customer actually received. The link is the last one the public app will serve —
 * a FAILED or PENDING attempt never reached the client, and a re-send mints a token of its own.
 */
export function QuoteDeliveryCard({ detail, onSent }: QuoteDeliveryCardProps) {
  const t = useTranslations('rfqs.detail.quoteCard');
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const quote = detail.quote;
  const version = detail.version;
  if (quote === null || version === null) return null;

  const canSendQuote = quote.current_status === 'QUOTED';

  // The API orders deliveries newest-first, so the first live one is the link the client holds.
  const liveDelivery = (detail.deliveries ?? []).find(
    (delivery) => LIVE_TRACKING_STATUSES.has(delivery.tracking_status) && delivery.public_url,
  );

  async function copyLink(url: string) {
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      return;
    }
    setCopiedUrl(url);
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-x-4">
          <div className="flex min-w-0 flex-col gap-y-1">
            <CardTitle>
              {t('title', {
                reference: formatRfqReference(detail.rfq.quote_number) ?? t('numberPending'),
              })}
            </CardTitle>
            <CardDescription>{t('version', { version: version.version_number })}</CardDescription>
          </div>
          <RfqStatusBadge status={normalizeRfqStatus(quote.current_status)} size="sm" />
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-2">
        {liveDelivery ? (
          <>
            <span className="text-paragraph-xs-medium text-foreground-muted">{t('linkLabel')}</span>
            <span
              aria-label={t('linkLabel')}
              className="break-all text-paragraph-sm text-foreground"
            >
              {liveDelivery.public_url}
            </span>
            <div className="flex gap-x-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => copyLink(liveDelivery.public_url)}
              >
                {copiedUrl === liveDelivery.public_url ? (
                  <CheckIcon className="size-3.5" />
                ) : (
                  <CopyIcon className="size-3.5" />
                )}
                {copiedUrl === liveDelivery.public_url ? t('copiedLink') : t('copyLink')}
              </Button>
              <Button asChild variant="outline" size="sm">
                <a href={liveDelivery.public_url} target="_blank" rel="noopener noreferrer">
                  <ExternalLinkIcon className="size-3.5" />
                  {t('openQuote')}
                </a>
              </Button>
            </div>
          </>
        ) : (
          <p className="text-paragraph-sm text-foreground-muted">{t('unavailable')}</p>
        )}
      </CardContent>
      {canSendQuote && (
        <CardFooter className="justify-end">
          <SendQuoteDialog detail={detail} branchId={detail.rfq.branch_id} onSent={onSent} />
        </CardFooter>
      )}
    </Card>
  );
}
