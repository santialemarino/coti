'use client';

import { useCallback, useEffect, useRef, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { ArchiveIcon } from 'lucide-react';
import { useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Callout, ConfirmDialog, PendingButton } from '@repo/ui/components';
import { followupDue } from '@/app/(protected)/rfqs/_components/followup-badge';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { PublicRequestCard } from '@/app/(protected)/rfqs/[id]/_components/public-request-card';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type { QuoteDiscountResponse, QuoteItemResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import { formatRfqReference, normalizeRfqStatus } from '@/lib/api/rfqs';
import {
  discardAIChanges,
  fetchRfqDetail,
  generateQuote,
  setQuoteArchived,
} from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { ClientAssociationCard } from './client-association-card';
import { InvoiceCard } from './invoice-card';
import { QuoteDeliveryCard } from './quote-delivery-card';
import { RfqChangeRequestCard } from './rfq-change-request-card';
import { RfqConversation } from './rfq-conversation';
import { RfqDetailHeader } from './rfq-detail-header';
import { RfqItemsTable } from './rfq-items-table';
import { RfqStatusTimeline } from './rfq-status-timeline';

interface RfqDetailViewProps {
  detail: RfqDetailResponse;
  /* Whether the order's branch has no mailbox for a customer's reply to reach. */
  branchMissesEmail?: boolean;
}

// What the queue shows for the order's lines, recounted whenever this screen changes them.
function lineCounts(items: QuoteItemResponse[]): { itemCount: number; reviewCount: number } {
  return {
    itemCount: items.length,
    reviewCount: items.filter((item) => item.match_status !== 'MATCHED').length,
  };
}

export function RfqDetailView({
  detail: initialDetail,
  branchMissesEmail = false,
}: RfqDetailViewProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const tCommon = useTranslations('common');
  const message = useApiErrorMessage('rfqs.detail.items');
  const { updateRecord } = useRfqList();
  const reduced = useReducedMotion();
  const [detail, setDetail] = useState(initialDetail);
  const [items, setItems] = useState<QuoteItemResponse[]>(initialDetail.items);
  const [discounts, setDiscounts] = useState<QuoteDiscountResponse[]>(
    initialDetail.discounts ?? [],
  );
  const [shownInitial, setShownInitial] = useState(initialDetail);
  const [generating, startGenerate] = useTransition();
  const [unarchiving, startUnarchive] = useTransition();
  const [discardOpen, setDiscardOpen] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  // The sale's client: the quote's own, then whatever the association card changes it to.
  const [clientId, setClientId] = useState<string | null | undefined>(
    initialDetail.quote?.client_id,
  );
  const rootRef = useRef<HTMLDivElement>(null);

  const quoteId = detail.quote?.id ?? null;
  // The order's own branch, which is what every write on this screen is scoped to.
  const branchId = detail.rfq.branch_id;
  const quoteStatus = detail.quote?.current_status ?? null;
  // Business status the seller sees: DRAFT (an internal quote_state) reads as GENERATED here.
  // A requested revision remains a mutable draft until the seller accepts its materials.
  const rfqStatus = normalizeRfqStatus(detail.rfq.status);
  // Every edit on an archived quote is refused until it is unarchived, so the screen offers none.
  const archived = detail.quote?.archived_at != null;
  const isDraft = !archived && (quoteStatus === 'DRAFT' || quoteStatus === 'CHANGE_REQUESTED');
  const latestClientAction = detail.client_actions?.at(-1);
  // A customer's messages reach the draft through the conversation handler, which leaves a note.
  const handlerNote = detail.changes_requested?.handler;
  const isCustomerChangeRequest =
    quoteStatus === 'CHANGE_REQUESTED' &&
    detail.version !== null &&
    (handlerNote != null ||
      (latestClientAction?.type === 'REQUEST_CHANGE' &&
        latestClientAction.version_number === detail.version.version_number - 1));
  const canDiscardAIChanges = !archived && detail.conversation?.can_discard_ai_changes === true;
  const followupOrigin = rfqStatus === 'SENT' ? 'customer' : 'seller';
  const showFollowup = followupDue(
    detail.rfq.needs_followup,
    rfqStatus,
    detail.rfq.archived_at != null,
  );
  const statusKey = `${rfqStatus}:${quoteStatus}`;
  const shownStatusKey = useRef(statusKey);

  // A server refresh hands down a new detail; adopt it rather than keep the first one forever.
  if (initialDetail !== shownInitial) {
    setShownInitial(initialDetail);
    setDetail(initialDetail);
    setItems(initialDetail.items);
    setDiscounts(initialDetail.discounts ?? []);
  }

  /*
   * A transition swaps the cards above the items in place, so a seller who acted from the bottom of
   * the page is left facing blank space. Only a status change scrolls; editing lines never does.
   */
  useEffect(() => {
    if (shownStatusKey.current === statusKey) return;
    shownStatusKey.current = statusKey;
    rootRef.current?.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'start' });
  }, [statusKey, reduced]);

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
      updateRecord(fresh.rfq.id, lineCounts(fresh.items));
    } catch {
      // The mutation itself succeeded; a failed refetch must not look like the write died.
      toast.error(t('detail.items.toast.error'));
    }
  }, [detail.rfq.id, quoteId, t, updateRecord]);

  const handleItemsChange = useCallback(
    (newItems: QuoteItemResponse[]) => {
      setItems(newItems);
      updateRecord(detail.rfq.id, lineCounts(newItems));
    },
    [detail.rfq.id, updateRecord],
  );

  function handleUnarchive() {
    if (!quoteId) return;
    const reference = formatRfqReference(detail.rfq.quote_number) ?? t('list.numberPending');
    startUnarchive(async () => {
      try {
        await setQuoteArchived(quoteId, false, branchId);
        setDetail((prev) => ({
          ...prev,
          rfq: { ...prev.rfq, archived_at: null },
          quote: prev.quote ? { ...prev.quote, archived_at: null } : prev.quote,
        }));
        updateRecord(detail.rfq.id, { archived: false });
        toast.success(t('list.toast.unarchived', { id: reference }));
        router.refresh();
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
      }
    });
  }

  async function handleDiscard() {
    if (!quoteId) return;
    const reference = formatRfqReference(detail.rfq.quote_number) ?? t('list.numberPending');
    setDiscarding(true);
    try {
      await discardAIChanges(quoteId, branchId);
      toast.success(t('detail.callouts.changeRequested.discard.success', { reference }));
      setDiscardOpen(false);
      await refreshDetail();
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setDiscarding(false);
    }
  }

  function handleGenerate() {
    if (!quoteId) return;
    startGenerate(async () => {
      try {
        const result = await generateQuote(quoteId, branchId);
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
          ...lineCounts(result.items),
          needsFollowup: result.quote.needs_followup,
          followupFlaggedAt: result.quote.followup_flagged_at,
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
    <div ref={rootRef} className="flex flex-col gap-y-4 scroll-mt-20">
      <RfqDetailHeader detail={detail} />

      <QuoteDeliveryCard
        detail={detail}
        onSent={refreshDetail}
        branchMissesEmail={branchMissesEmail}
      />

      {archived ? (
        <Callout
          tone="info"
          icon={ArchiveIcon}
          title={t('detail.callouts.archived.title')}
          action={
            <PendingButton
              type="button"
              variant="outline"
              size="sm"
              onClick={handleUnarchive}
              pending={unarchiving}
              pendingLabel={t('detail.callouts.archived.unarchiving')}
            >
              {t('detail.callouts.archived.unarchive')}
            </PendingButton>
          }
        >
          {t('detail.callouts.archived.description')}
        </Callout>
      ) : null}

      <RfqStatusTimeline detail={detail} />

      {detail.publicRequest ? <PublicRequestCard request={detail.publicRequest} /> : null}

      {showFollowup ? (
        <Callout tone="followup" title={t('detail.callouts.followup.title')}>
          {detail.rfq.followup_flagged_at
            ? t(`detail.callouts.followup.${followupOrigin}`, {
                date: fmt.date(detail.rfq.followup_flagged_at),
              })
            : t(`detail.callouts.followup.${followupOrigin}Undated`)}
        </Callout>
      ) : null}

      {rfqStatus === 'ACCEPTED' && (
        <>
          <Callout tone="success" title={t('detail.callouts.accepted.title')}>
            {t('detail.callouts.accepted.description')}
          </Callout>
          {quoteId && !archived ? (
            <ClientAssociationCard
              quoteId={quoteId}
              branchId={branchId}
              onClientChange={setClientId}
            />
          ) : null}
          {quoteId ? (
            <InvoiceCard
              quoteId={quoteId}
              branchId={branchId}
              clientId={clientId}
              readOnly={archived}
            />
          ) : null}
        </>
      )}

      {rfqStatus === 'REJECTED' && (
        <Callout tone="danger" title={t('detail.callouts.rejected.title')}>
          {detail.version?.comment
            ? t('detail.callouts.rejected.withReason', { reason: detail.version.comment })
            : t('detail.callouts.rejected.description')}
        </Callout>
      )}

      {rfqStatus === 'CHANGE_REQUESTED' && quoteId && detail.changes_requested ? (
        <RfqChangeRequestCard
          diff={detail.changes_requested}
          variant={isCustomerChangeRequest ? 'customer-request' : 'seller-revision'}
          conversation={detail.conversation}
          quoteId={quoteId}
          branchId={branchId}
          canDiscard={canDiscardAIChanges}
          onDiscard={() => setDiscardOpen(true)}
          onChanged={refreshDetail}
        />
      ) : (
        <>
          {rfqStatus === 'CHANGE_REQUESTED' && (
            <Callout
              tone="warning"
              title={t(
                isCustomerChangeRequest
                  ? 'detail.callouts.changeRequested.title'
                  : 'detail.callouts.sellerRevision.title',
              )}
            >
              {isCustomerChangeRequest
                ? t('detail.callouts.changeRequested.description')
                : t('detail.callouts.sellerRevision.description')}
            </Callout>
          )}
          {quoteId && detail.conversation ? (
            <RfqConversation
              conversation={detail.conversation}
              quoteId={quoteId}
              branchId={branchId}
              onChanged={refreshDetail}
            />
          ) : null}
        </>
      )}

      {rfqStatus === 'SENT' && detail.quote?.expires_at && (
        <Callout tone="info" title={t('detail.callouts.sent.title')}>
          {t('detail.callouts.sent.description', {
            date: fmt.date(detail.quote.expires_at),
          })}
        </Callout>
      )}

      <RfqItemsTable
        quoteId={quoteId}
        quoteStatus={quoteStatus}
        branchId={branchId}
        items={items}
        discounts={discounts}
        onItemsChange={handleItemsChange}
        onDiscountsChange={setDiscounts}
        onRefresh={refreshDetail}
        readOnly={archived}
      />

      <ConfirmDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        entity={canDiscardAIChanges ? quoteId : null}
        title={t('detail.callouts.changeRequested.discard.title')}
        description={() => t('detail.callouts.changeRequested.discard.description')}
        onConfirm={handleDiscard}
        pending={discarding}
        tone="danger"
        labels={{
          confirm: t('detail.callouts.changeRequested.discard.confirm'),
          pending: t('detail.callouts.changeRequested.discard.pending'),
          cancel: tCommon('actions.cancel'),
        }}
      />

      {isDraft && quoteId && (
        <div className="flex justify-end">
          <PendingButton
            type="button"
            onClick={handleGenerate}
            pending={generating}
            pendingLabel={t('detail.items.generating')}
          >
            {t('detail.items.generate')}
          </PendingButton>
        </div>
      )}
    </div>
  );
}
