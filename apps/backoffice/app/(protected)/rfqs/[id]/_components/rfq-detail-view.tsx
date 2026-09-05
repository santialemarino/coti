'use client';

import { useCallback, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Callout, PendingButton } from '@repo/ui/components';
import type { QuoteDiscountResponse, QuoteItemResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { generateQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { RfqChangeDiff } from './rfq-change-diff';
import { RfqDetailHeader } from './rfq-detail-header';
import { RfqItemsTable } from './rfq-items-table';
import { RfqStatusTimeline } from './rfq-status-timeline';

interface RfqDetailViewProps {
  detail: RfqDetailResponse;
}

/*
 * Discounts are not persisted by the backend yet (US-38), so a real response carries none. To
 * exercise the discount summary + editing, surface a couple of example rows while the seller is
 * still composing (QUOTED / CHANGE_REQUESTED). Frozen states keep none: a sent quote's totals are
 * what were sent, and fabricating discounts there would contradict the version.total in the
 * timeline. Editing stays client-side for now.
 */
const EXAMPLE_DISCOUNT_ROWS: Omit<QuoteDiscountResponse, 'amount'>[] = [
  {
    id: 'example-promo-qty',
    quote_version_id: '',
    promotion_id: 'e0000000-0000-4000-8000-000000000001',
    promotion_name: 'Cemento por cantidad',
    condition_type: 'QUANTITY_TIERED',
    scope: 'ITEM',
    origin: 'AUTOMATIC',
    suppressed_by_seller: false,
    created_at: '',
  },
  {
    id: 'example-promo-total',
    quote_version_id: '',
    promotion_id: 'e0000000-0000-4000-8000-000000000002',
    promotion_name: 'Compra grande',
    condition_type: 'ON_TOTAL',
    scope: 'TOTAL',
    origin: 'AUTOMATIC',
    suppressed_by_seller: false,
    created_at: '',
  },
];

function seedExampleDiscounts(detail: RfqDetailResponse): QuoteDiscountResponse[] {
  if ((detail.discounts?.length ?? 0) > 0) return detail.discounts ?? [];

  const status = detail.quote?.current_status ?? '';
  const isComposing = status === 'QUOTED' || status === 'CHANGE_REQUESTED';
  if (!isComposing) return [];

  const total = Number(detail.version?.total ?? 0);
  if (!Number.isFinite(total) || total <= 0) return [];

  const quantityDiscount = Math.round(Math.min(total * 0.1, 200000) * 100) / 100;
  const totalDiscount = Math.round(total * 0.05 * 100) / 100;

  return [
    { ...EXAMPLE_DISCOUNT_ROWS[0]!, amount: String(quantityDiscount) },
    { ...EXAMPLE_DISCOUNT_ROWS[1]!, amount: String(totalDiscount) },
  ];
}

export function RfqDetailView({ detail: initialDetail }: RfqDetailViewProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const [detail, setDetail] = useState(initialDetail);
  const [items, setItems] = useState<QuoteItemResponse[]>(initialDetail.items);
  const [discounts, setDiscounts] = useState<QuoteDiscountResponse[]>(() =>
    seedExampleDiscounts(initialDetail),
  );
  const [generating, startGenerate] = useTransition();

  const quoteId = detail.quote?.id ?? null;
  const quoteStatus = detail.quote?.current_status ?? null;
  // Business status the seller sees: DRAFT (an internal quote_state) reads as GENERATED here.
  // isDraft below stays raw so the generate button only surfaces while the quote is really DRAFT.
  const rfqStatus = normalizeRfqStatus(detail.rfq.status);
  const isDraft = quoteStatus === 'DRAFT';

  const handleItemsChange = useCallback((newItems: QuoteItemResponse[]) => {
    setItems(newItems);
  }, []);

  const handleDiscountsChange = useCallback((newDiscounts: QuoteDiscountResponse[]) => {
    setDiscounts(newDiscounts);
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
            onDiscountsChange={handleDiscountsChange}
          />
        </>
      ) : (
        <RfqItemsTable
          quoteId={quoteId}
          quoteStatus={quoteStatus}
          items={items}
          discounts={discounts}
          onItemsChange={handleItemsChange}
          onDiscountsChange={handleDiscountsChange}
        />
      )}

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
