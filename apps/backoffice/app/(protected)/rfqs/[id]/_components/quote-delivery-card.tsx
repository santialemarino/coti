'use client';

import { ExternalLinkIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  CopyButton,
} from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { SetupNotice } from '@/components/setup-notice';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { formatRfqReference, normalizeRfqStatus } from '@/lib/api/rfqs';
import { QuoteLifecycleActions } from './quote-lifecycle-actions';
import { SendQuoteDialog } from './send-quote-dialog';

// The public endpoint only resolves tokens in these states, so any other state would hand the seller
// a link the customer cannot open.
const LIVE_TRACKING_STATUSES = new Set(['SENT', 'DELIVERED', 'VIEWED']);

interface QuoteDeliveryCardProps {
  detail: RfqDetailResponse;
  onSent: () => Promise<void>;
  /* Whether the order's branch has no mailbox for a customer's reply to reach. */
  branchMissesEmail?: boolean;
}

/*
 * The quotation itself, as a thing the seller can act on: its reference, its version and status,
 * and the link the customer actually received. The link is the last one the public app will serve —
 * a FAILED or PENDING attempt never reached the client, and a re-send mints a token of its own.
 */
export function QuoteDeliveryCard({
  detail,
  onSent,
  branchMissesEmail = false,
}: QuoteDeliveryCardProps) {
  const t = useTranslations('rfqs.detail.quoteCard');
  const { isAdmin } = useRfqList();

  const quote = detail.quote;
  const version = detail.version;
  if (quote === null || version === null) return null;

  const canAct = quote.archived_at === null;
  const canSendQuote = canAct && quote.current_status === 'QUOTED';

  // The API orders deliveries newest-first, so the first live one is the link the client holds.
  const liveDelivery = (detail.deliveries ?? []).find(
    (delivery) => LIVE_TRACKING_STATUSES.has(delivery.tracking_status) && delivery.public_url,
  );

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
      <CardContent className="flex flex-col gap-y-4">
        {canSendQuote && branchMissesEmail ? (
          <SetupNotice issue="BRANCH_EMAIL" isAdmin={isAdmin} />
        ) : null}
        {liveDelivery ? (
          <>
            <div className="flex flex-col gap-y-1">
              <span className="text-paragraph-xs-medium text-foreground-muted">
                {t('linkLabel')}
              </span>
              <span
                aria-label={t('linkLabel')}
                className="break-all text-paragraph-sm text-foreground"
              >
                {liveDelivery.public_url}
              </span>
            </div>
            <div className="flex gap-x-2">
              <CopyButton
                value={liveDelivery.public_url}
                labels={{ copy: t('copyLink'), copied: t('copiedLink') }}
                onCopyError={() => toast.error(t('copyFailed'))}
              />
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
      {canAct ? (
        /* Both action sets stay mounted across every transition, so a dialog outlives the status
           that offered it; the footer hides itself while neither has a button to show. */
        <CardFooter className="justify-end gap-x-2 empty:hidden">
          <SendQuoteDialog
            detail={detail}
            branchId={detail.rfq.branch_id}
            onSent={onSent}
            showTrigger={canSendQuote}
            branchMissesEmail={branchMissesEmail}
          />
          <QuoteLifecycleActions
            quoteId={quote.id}
            branchId={detail.rfq.branch_id}
            status={quote.current_status}
            onChanged={onSent}
          />
        </CardFooter>
      ) : null}
    </Card>
  );
}
