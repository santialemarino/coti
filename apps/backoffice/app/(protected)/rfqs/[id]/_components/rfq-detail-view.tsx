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
import { ComboSuggestions } from '@/app/(protected)/rfqs/[id]/_components/combo-suggestions';
import { PublicRequestCard } from '@/app/(protected)/rfqs/[id]/_components/public-request-card';
import { SendQuoteDialog } from '@/app/(protected)/rfqs/[id]/_components/send-quote-dialog';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { useQueuePolling } from '@/hooks/use-queue-polling';
import type { ClientProfile } from '@/lib/api/client-profiles';
import { errorCodeOf } from '@/lib/api/errors';
import type { QuoteDiscountResponse, QuoteItemResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import { formatRfqReference, mapListItem, normalizeRfqStatus } from '@/lib/api/rfqs';
import {
  discardAIChanges,
  fetchRfqDetail,
  generateQuote,
  setFailedRfqArchived,
  setQuoteArchived,
} from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { AcceptSentVersionButton } from './accept-sent-version-button';
import { CustomerReplyCallout } from './customer-reply-callout';
import { FollowupCallout } from './followup-callout';
import { InvoiceCard } from './invoice-card';
import { QuoteDeliveryCard } from './quote-delivery-card';
import { ReplyAcceptanceCallout } from './reply-acceptance-callout';
import { RfqChangeRequestCard } from './rfq-change-request-card';
import { RfqClientCard } from './rfq-client-card';
import { RfqConversation } from './rfq-conversation';
import { RfqDetailHeader } from './rfq-detail-header';
import { RfqItemsTable } from './rfq-items-table';
import { RfqOriginalRequest } from './rfq-original-request';
import { RfqStatusTimeline } from './rfq-status-timeline';
import { SaleClosureCard } from './sale-closure-card';
import { SplitOrderCallout } from './split-order-callout';

interface RfqDetailViewProps {
  detail: RfqDetailResponse;
  clientProfile?: ClientProfile | null;
  /* Whether the order's branch has no mailbox for a customer's reply to reach. */
  branchMissesEmail?: boolean;
  /* Whether the corralón invoices with Coti; off, an accepted order shows no invoice. */
  invoicingEnabled: boolean;
  /* Whether «Marcar cobrado» starts with «Facturar también» ticked. */
  invoiceOnPayment?: boolean;
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
  clientProfile,
  branchMissesEmail = false,
  invoicingEnabled,
  invoiceOnPayment = false,
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
  const [archiving, startArchive] = useTransition();
  const [discardOpen, setDiscardOpen] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  // Bumped when closing the sale may have issued its invoice, so the invoice card reads it again.
  const [invoiceRevision, setInvoiceRevision] = useState(0);
  const clientId = detail.quote?.client_id;
  const rootRef = useRef<HTMLDivElement>(null);

  const quoteId = detail.quote?.id ?? null;
  // The order's own branch, which is what every write on this screen is scoped to.
  const branchId = detail.rfq.branch_id;
  const quoteStatus = detail.quote?.current_status ?? null;
  // Business status the seller sees: DRAFT (an internal quote_state) reads as GENERATED here.
  // A requested revision remains a mutable draft until the seller accepts its materials.
  const rfqStatus = normalizeRfqStatus(detail.rfq.status);
  // Every edit on an archived quote is refused until it is unarchived, so the screen offers none.
  const archived = detail.rfq.archived_at != null;
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
  // Raising the flag is not activity and any later write clears it, so a flagged quote's
  // updated_at is when it went quiet.
  const followupQuietSince = detail.quote?.updated_at ?? null;
  const showFollowup = followupDue(
    detail.rfq.needs_followup,
    rfqStatus,
    detail.rfq.archived_at != null,
  );
  const reference = formatRfqReference(detail.rfq.quote_number) ?? t('list.numberPending');
  const { acceptanceUnconfirmed = false, customerReply } = mapListItem(detail.rfq);
  // An acceptance read from a reply can be undone while nothing has closed the sale; the API checks
  // the send too and answers QUOTE_ACCEPTANCE_NOT_UNDOABLE when it has expired.
  const replyUndoable =
    detail.accepted_by_reply === true &&
    !detail.rfq.paid_at &&
    !detail.rfq.invoiced &&
    !detail.rfq.invoice_pending &&
    !detail.rfq.invoice_waived_at;
  // The version the customer received: the one before an open draft, or the current one.
  const sentVersion = detail.version
    ? detail.version.is_immutable
      ? detail.version.version_number
      : detail.version.version_number - 1
    : 0;
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
  const refreshDetail = useCallback(
    async (silent = false) => {
      try {
        const fresh = await fetchRfqDetail(detail.rfq.id);
        if (
          fresh.quote?.client_id &&
          (fresh.quote.client_id !== clientId ||
            fresh.quote.current_status !== quoteStatus ||
            (fresh.rfq.archived_at != null) !== archived)
        ) {
          router.refresh();
        }
        setDetail(fresh);
        setItems(fresh.items);
        setDiscounts(fresh.discounts ?? []);
        updateRecord(fresh.rfq.id, { ...mapListItem(fresh.rfq), ...lineCounts(fresh.items) });
      } catch {
        // The mutation itself succeeded; a failed refetch must not look like the write died.
        if (!silent) toast.error(t('detail.items.toast.error'));
      }
    },
    [archived, clientId, detail.rfq.id, quoteStatus, router, t, updateRecord],
  );

  useQueuePolling(() => refreshDetail(true), rfqStatus === 'RECEIVED');

  const handleItemsChange = useCallback(
    (newItems: QuoteItemResponse[]) => {
      setItems(newItems);
      updateRecord(detail.rfq.id, lineCounts(newItems));
    },
    [detail.rfq.id, updateRecord],
  );

  async function handleClosureChanged() {
    await refreshDetail();
    setInvoiceRevision((revision) => revision + 1);
  }

  function handleUnarchive() {
    startUnarchive(() => setArchived(false));
  }

  function handleArchive() {
    startArchive(() => setArchived(true));
  }

  async function setArchived(next: boolean) {
    try {
      if (quoteId) await setQuoteArchived(quoteId, next, branchId);
      else await setFailedRfqArchived(detail.rfq.id, next, branchId);
      const archivedAt = next ? new Date().toISOString() : null;
      setDetail((prev) => ({
        ...prev,
        rfq: { ...prev.rfq, archived_at: archivedAt },
        quote: prev.quote ? { ...prev.quote, archived_at: archivedAt } : prev.quote,
      }));
      updateRecord(detail.rfq.id, { archived: next });
      toast.success(t(next ? 'list.toast.archived' : 'list.toast.unarchived', { id: reference }));
      router.refresh();
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  async function handleDiscard() {
    if (!quoteId) return;
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

      {detail.split_from && !archived && (quoteId || rfqStatus === 'FAILED') ? (
        <SplitOrderCallout
          split={detail.split_from}
          createdAt={detail.rfq.created_at}
          archiving={archiving}
          onArchive={handleArchive}
        />
      ) : null}

      <RfqStatusTimeline detail={detail} />
      {clientId && clientProfile && initialDetail.quote?.client_id === clientId ? (
        <RfqClientCard key={clientProfile.client.id} profile={clientProfile} />
      ) : null}

      {detail.publicRequest ? <PublicRequestCard request={detail.publicRequest} /> : null}
      <RfqOriginalRequest
        rfqId={detail.rfq.id}
        branchId={branchId}
        text={detail.publicRequest?.originalText || detail.originalText}
      />

      {showFollowup ? (
        <FollowupCallout
          quoteId={quoteId}
          branchId={branchId}
          origin={followupOrigin}
          quietSince={followupQuietSince}
          detail={detail}
          onSent={refreshDetail}
        />
      ) : null}

      {rfqStatus === 'SENT' && customerReply && quoteId && !archived ? (
        <CustomerReplyCallout
          quoteId={quoteId}
          branchId={branchId}
          reference={reference}
          reply={customerReply}
          onChanged={refreshDetail}
        />
      ) : null}

      {rfqStatus === 'ACCEPTED' && (
        <>
          {quoteId && (acceptanceUnconfirmed || replyUndoable) ? (
            <ReplyAcceptanceCallout
              quoteId={quoteId}
              branchId={branchId}
              reference={reference}
              unconfirmed={acceptanceUnconfirmed}
              undoable={replyUndoable}
              readOnly={archived}
              onChanged={refreshDetail}
            />
          ) : (
            <Callout tone="success" title={t('detail.callouts.accepted.title')}>
              {t('detail.callouts.accepted.description')}
            </Callout>
          )}
          {quoteId ? (
            <SaleClosureCard
              quoteId={quoteId}
              branchId={branchId}
              reference={reference}
              paidAt={detail.rfq.paid_at}
              paidBy={detail.rfq.paid_by}
              saleComplete={detail.rfq.sale_complete}
              invoicingEnabled={invoicingEnabled}
              invoiceOnPayment={invoiceOnPayment}
              invoiceWaived={detail.rfq.invoice_waived_at != null}
              readOnly={archived || acceptanceUnconfirmed}
              onChanged={handleClosureChanged}
            />
          ) : null}
          {quoteId && invoicingEnabled ? (
            <InvoiceCard
              quoteId={quoteId}
              branchId={branchId}
              clientId={clientId}
              readOnly={archived || acceptanceUnconfirmed}
              reference={reference}
              waiver={
                detail.rfq.invoice_waived_at
                  ? { at: detail.rfq.invoice_waived_at, by: detail.rfq.invoice_waived_by }
                  : null
              }
              onWaiverChanged={refreshDetail}
              revision={invoiceRevision}
            />
          ) : null}
        </>
      )}

      {rfqStatus === 'REJECTED' && (
        <Callout tone="danger" title={t('detail.callouts.rejected.title')}>
          {latestClientAction?.type === 'REJECT' &&
          latestClientAction.version_id === detail.version?.id
            ? latestClientAction.comment || t('detail.callouts.rejected.noReason')
            : t('detail.callouts.rejected.description')}
        </Callout>
      )}

      {detail.client_actions
        ?.filter((action) => action.type === 'REJECT')
        .map((action) => (
          <Callout
            key={action.id}
            tone="info"
            title={
              action.reopened_at
                ? t('detail.callouts.rejected.historyReopened', {
                    version: action.version_number,
                    date: fmt.timestamp(action.created_at),
                    reopened: fmt.timestamp(action.reopened_at),
                  })
                : t('detail.callouts.rejected.history', {
                    version: action.version_number,
                    date: fmt.timestamp(action.created_at),
                  })
            }
          >
            <p className="whitespace-pre-wrap break-words">
              {action.comment || t('detail.callouts.rejected.noReason')}
            </p>
          </Callout>
        ))}

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
              accepted={rfqStatus === 'ACCEPTED'}
            />
          ) : null}
        </>
      )}

      {rfqStatus === 'CHANGE_REQUESTED' && quoteId && !archived && sentVersion > 0 ? (
        <div className="flex justify-end">
          <AcceptSentVersionButton
            quoteId={quoteId}
            branchId={branchId}
            reference={reference}
            sentVersion={sentVersion}
            onChanged={refreshDetail}
          />
        </div>
      ) : null}

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
        readOnly={archived || detail.version?.is_immutable === true}
      />

      {quoteId && quoteStatus === 'QUOTED' && !archived ? (
        <ComboSuggestions
          quoteId={quoteId}
          branchId={branchId}
          items={items}
          onAdded={refreshDetail}
        />
      ) : null}

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

      {quoteId && quoteStatus === 'QUOTED' && !archived ? (
        <div className="flex justify-end">
          <SendQuoteDialog
            detail={detail}
            branchId={branchId}
            onSent={refreshDetail}
            branchMissesEmail={branchMissesEmail}
          />
        </div>
      ) : null}

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
