'use client';

import { useCallback, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Callout, PendingButton } from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
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
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const message = useApiErrorMessage('rfqs.detail.items');
  const { activeBranchId, updateRecord } = useRfqList();
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
    if (!activeBranchId) {
      toast.error(t('detail.items.toast.branchRequired'));
      return;
    }
    startGenerate(async () => {
      try {
        const result = await generateQuote(quoteId);
        const status = normalizeRfqStatus(result.quote.current_status);
        toast.success(t('detail.items.toast.generated'));
        setDetail((prev) => ({
          ...prev,
          rfq: { ...prev.rfq, status, total: result.version.total },
          quote: result.quote,
          version: result.version,
        }));
        setItems(result.items);
        updateRecord(detail.rfq.id, {
          archived: result.quote.archived_at != null,
          itemCount: result.items.length,
          needsFollowup: result.quote.needs_followup,
          status,
          total: result.version.total,
        });
        router.refresh();
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
        router.refresh();
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
            onDiscountsChange={setDiscounts}
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
          onDiscountsChange={setDiscounts}
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
