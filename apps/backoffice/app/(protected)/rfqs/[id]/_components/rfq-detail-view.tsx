'use client';

import { useCallback, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Callout, PendingButton } from '@repo/ui/components';
import type { QuoteDiscountResponse, QuoteItemResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { fetchRfqDetail, generateQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { RfqChangeDiff } from './rfq-change-diff';
import { RfqDetailHeader } from './rfq-detail-header';
import { RfqItemsTable } from './rfq-items-table';
import { RfqStatusTimeline } from './rfq-status-timeline';
import { SendQuoteDialog } from './send-quote-dialog';

interface RfqDetailViewProps {
  detail: RfqDetailResponse;
}

export function RfqDetailView({ detail: initialDetail }: RfqDetailViewProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const [detail, setDetail] = useState(initialDetail);
  const [items, setItems] = useState<QuoteItemResponse[]>(initialDetail.items);
  const [discounts, setDiscounts] = useState<QuoteDiscountResponse[]>(
    initialDetail.discounts ?? [],
  );
  const [generating, startGenerate] = useTransition();

  const quoteId = detail.quote?.id ?? null;
  const quoteStatus = detail.quote?.current_status ?? null;
  // Business status the seller sees: DRAFT (an internal quote_state) reads as GENERATED here.
  // isDraft below stays raw so the generate button only surfaces while the quote is really DRAFT.
  const rfqStatus = normalizeRfqStatus(detail.rfq.status);
  const isDraft = quoteStatus === 'DRAFT';
  // The send flow is one screen for every review-ready status: same modal, same mock handoff.
  const canSendQuote = quoteStatus === 'QUOTED' || quoteStatus === 'CHANGE_REQUESTED';

  /*
   * Reconcile the screen against the backend after a mutation. The discount endpoints
   * recompute the version total, so item subtotals, discounts and version.total all come
   * back together and the timeline and summary cannot drift from what is persisted.
   */
  const refreshDetail = useCallback(async () => {
    if (!quoteId) return;
    try {
      const fresh = await fetchRfqDetail(detail.rfq.id);
      setDetail(fresh);
      setItems(fresh.items);
      setDiscounts(fresh.discounts ?? []);
    } catch {
      // The mutation itself succeeded; a failed refetch must not look like the write died.
      toast.error(t('detail.items.toast.error'));
    }
  }, [detail.rfq.id, quoteId, t]);

  const handleItemsChange = useCallback((newItems: QuoteItemResponse[]) => {
    setItems(newItems);
  }, []);

  function handleGenerate() {
    if (!quoteId) return;
    startGenerate(async () => {
      try {
        const result = await generateQuote(quoteId);
        toast.success(t('detail.items.toast.generated'));
        setDetail((prev) => ({
          ...prev,
          quote: result.quote as RfqDetailResponse['quote'],
          version: result.version as RfqDetailResponse['version'],
        }));
        setItems(result.items);
      } catch {
        toast.error(t('detail.items.toast.error'));
      }
    });
  }

  return (
    <div className="flex flex-col gap-y-4">
      <RfqDetailHeader detail={detail} />

      <RfqStatusTimeline detail={detail} />

      <div className="border-t border-border" />

      {rfqStatus === 'ACCEPTED' && (
        <Callout tone="success" title={t('detail.callouts.accepted.title')}>
          {t('detail.callouts.accepted.description')}
        </Callout>
      )}

      {rfqStatus === 'REJECTED' && (
        <Callout tone="danger" title={t('detail.callouts.rejected.title')}>
          {detail.version?.comment
            ? t('detail.callouts.rejected.withReason', { reason: detail.version.comment })
            : t('detail.callouts.rejected.description')}
        </Callout>
      )}

      {rfqStatus === 'CHANGE_REQUESTED' && (
        <Callout tone="warning" title={t('detail.callouts.changeRequested.title')}>
          {detail.version?.comment
            ? t('detail.callouts.changeRequested.withReason', { reason: detail.version.comment })
            : t('detail.callouts.changeRequested.description')}
        </Callout>
      )}

      {rfqStatus === 'SENT' && detail.quote?.expires_at && (
        <Callout tone="info" title={t('detail.callouts.sent.title')}>
          {t('detail.callouts.sent.description', {
            date: fmt.date(detail.quote.expires_at),
          })}
        </Callout>
      )}

      {rfqStatus === 'CHANGE_REQUESTED' && detail.changes_requested ? (
        <>
          <RfqChangeDiff diff={detail.changes_requested} />
          <RfqItemsTable
            quoteId={quoteId}
            quoteStatus={quoteStatus}
            items={items}
            discounts={discounts}
            onItemsChange={handleItemsChange}
            onRefresh={refreshDetail}
          />
        </>
      ) : (
        <RfqItemsTable
          quoteId={quoteId}
          quoteStatus={quoteStatus}
          items={items}
          discounts={discounts}
          onItemsChange={handleItemsChange}
          onRefresh={refreshDetail}
        />
      )}

      {(isDraft || canSendQuote) && quoteId && (
        <div className="flex justify-end">
          {isDraft && (
            <PendingButton
              type="button"
              onClick={handleGenerate}
              pending={generating}
              pendingLabel={t('detail.items.generating')}
            >
              {t('detail.items.generate')}
            </PendingButton>
          )}
          {canSendQuote && <SendQuoteDialog detail={detail} />}
        </div>
      )}
    </div>
  );
}
