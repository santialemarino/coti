'use client';

import { useMemo, useState } from 'react';
import { MinusIcon, PackageOpenIcon, PencilIcon, PlusIcon, TrashIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  RowActionButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { AmountInput } from '@/components/amount-input';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { CatalogProduct } from '@/lib/api/catalog';
import { errorCodeOf } from '@/lib/api/errors';
import type {
  ConfidenceLevel,
  CreateDiscountBody,
  QuoteDiscountResponse,
  QuoteItemResponse,
} from '@/lib/api/rfqs';
import {
  addDiscount,
  addQuoteItem,
  deleteDiscount,
  deleteQuoteItem,
  updateDiscount,
  updateQuoteItem,
} from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { DiscountDialog } from './discount-dialog';
import { ProductSearchDialog } from './product-search-dialog';

/*
 * Capability matrix keyed on the raw quote.current_status. GENERATED reads as DRAFT here (the
 * backend-only state while a draft quote is being worked); the seller never sees DRAFT as a
 * status, it only decides what is editable. Gaps are deliberate and map 1:1 to the status rules:
 *   GENERATED (DRAFT)      products editable, no prices shown yet
 *   CHANGE_REQUESTED       products editable, provisional prices shown
 *   QUOTED                 prices (and discounts) editable
 *   SENT/ACCEPTED/REJECTED  read-only, prices shown
 */
const PRODUCT_EDIT_STATUSES = new Set(['DRAFT', 'CHANGE_REQUESTED']);
const PRICE_EDIT_STATUSES = new Set(['QUOTED']);
const PRICED_STATUSES = new Set(['CHANGE_REQUESTED', 'QUOTED', 'SENT', 'ACCEPTED', 'REJECTED']);

// A line quantity is a measured figure, not a count — half a cubic metre is a real order line.
// Both are NUMERIC(14,2) on the wire, so entry is capped where storage is.
const QUANTITY_DECIMALS = 2;
// Quotes are priced in the account's currency; the multi-currency catalogue is a later decision.
const ACCOUNT_CURRENCY = 'ARS';

// An inline text trigger: a colour shift and the underline mark focus as they mark hover, and the
// icon's bump confirms it.
const PRODUCT_TRIGGER =
  'group/product flex items-center gap-x-1 rounded-sm outline-none transition-colors duration-200 ease-out-soft hover:text-foreground hover:underline focus-visible:text-foreground focus-visible:underline text-paragraph-xs text-foreground-muted underline-offset-2';
const PRODUCT_TRIGGER_ICON = 'size-3 group-focus-visible/product:animate-focus-bump-soft';

interface RfqItemsTableProps {
  quoteId: string | null;
  quoteStatus: string | null;
  // The branch that owns this order. Every write is scoped to it rather than to the header
  // switcher, which is a filter over the list and sits on "todas" by default.
  branchId: string;
  items: QuoteItemResponse[];
  discounts: QuoteDiscountResponse[];
  onItemsChange: (items: QuoteItemResponse[]) => void;
  onDiscountsChange?: (discounts: QuoteDiscountResponse[]) => void;
  onRefresh?: () => Promise<void>;
}

function toQuantity(value: string): string {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return '1';
  return String(parsed);
}

const CONFIDENCE_TONES: Record<ConfidenceLevel, 'success' | 'warning' | 'danger'> = {
  HIGH: 'success',
  MEDIUM: 'warning',
  LOW: 'danger',
};

// The API reads the level off its own calibration, so the badge follows the backend's thresholds.
// A matched line with no level is one a person chose the product for.
function confidenceBadge(item: QuoteItemResponse): {
  tone: 'success' | 'warning' | 'danger' | 'neutral';
  label: string;
} {
  if (item.confidence_level) {
    return {
      tone: CONFIDENCE_TONES[item.confidence_level],
      label: item.confidence_level.toLowerCase(),
    };
  }
  return { tone: 'neutral', label: item.match_status === 'MATCHED' ? 'manual' : 'none' };
}

// The products matching offered for the line, best first. A flagged line leads with the product it
// already carries, so confirming it is one click rather than a search for what is on screen.
function suggestionsFor(item: QuoteItemResponse | undefined): CatalogProduct[] {
  if (!item) return [];
  const current: CatalogProduct[] =
    item.product_id && item.match_status !== 'MATCHED'
      ? [
          {
            id: item.product_id,
            code: item.product_code ?? '',
            name: item.product_name ?? '',
            unit: item.product_unit ?? '',
          },
        ]
      : [];
  const offered = [...item.alternatives]
    .filter((alternative) => alternative.product_id && alternative.product_id !== item.product_id)
    .sort((a, b) => a.rank - b.rank)
    .map((alternative) => ({
      id: alternative.product_id as string,
      code: alternative.code ?? '',
      name: alternative.canonical_name ?? '',
      unit: alternative.unit ?? '',
    }));
  return [...current, ...offered];
}

// The raw rule a seller-typed discount carries, e.g. "10 %" or "$ 1.500,00"; null when the
// row keeps no rule (engine-applied or pre-rule manual discounts).
function discountRuleLabel(
  discount: QuoteDiscountResponse,
  fmt: ReturnType<typeof useFormatters>,
): string | null {
  if (discount.action_value == null) return null;
  return discount.action_type === 'PERCENTAGE'
    ? `${fmt.value(Number(discount.action_value))} %`
    : fmt.currency(discount.action_value);
}

export function RfqItemsTable({
  quoteId,
  quoteStatus,
  branchId,
  items,
  discounts,
  onItemsChange,
  onDiscountsChange,
  onRefresh,
}: RfqItemsTableProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const tCommon = useTranslations('common');
  const message = useApiErrorMessage('rfqs.detail.items');
  const [searchOpen, setSearchOpen] = useState(false);
  // The line awaiting confirmation, and the one whose delete is in flight.
  const [pendingDelete, setPendingDelete] = useState<QuoteItemResponse | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [editingQuantity, setEditingQuantity] = useState<Record<string, string>>({});
  const [editingPrice, setEditingPrice] = useState<Record<string, string>>({});
  const [discountDialogOpen, setDiscountDialogOpen] = useState(false);
  const [editingDiscount, setEditingDiscount] = useState<QuoteDiscountResponse | null>(null);
  const [editingProductItemId, setEditingProductItemId] = useState<string | null>(null);

  const hasQuote = !!quoteId;
  const showPricing = hasQuote && PRICED_STATUSES.has(quoteStatus ?? '');
  const canEditProducts = hasQuote && PRODUCT_EDIT_STATUSES.has(quoteStatus ?? '');
  const canEditPrices = hasQuote && PRICE_EDIT_STATUSES.has(quoteStatus ?? '');
  const canEditDiscounts = canEditPrices;
  // Confidence belongs to the pre-quote review: visible before pricing is set, hidden once a
  // quote exists and the seller reasons about products and money.
  const showConfidence = !showPricing;
  const hasDiscounts = discounts.length > 0;

  const itemsSubtotal = useMemo(
    () => items.reduce((sum, item) => sum + Number(item.subtotal ?? 0), 0),
    [items],
  );
  const discountsTotal = useMemo(
    () =>
      discounts
        .filter((d) => !d.suppressed_by_seller)
        .reduce((sum, d) => sum + Number(d.amount), 0),
    [discounts],
  );
  const grandTotal = itemsSubtotal - discountsTotal;

  async function handleDiscountSave(body: CreateDiscountBody) {
    if (!quoteId) return;
    try {
      if (editingDiscount) {
        await updateDiscount(quoteId, editingDiscount.id, branchId, body);
        toast.success(t('detail.items.discounts.toast.updated', { name: body.description }));
      } else {
        await addDiscount(quoteId, branchId, body);
        toast.success(t('detail.items.discounts.toast.added', { name: body.description }));
      }
      await onRefresh?.();
      setDiscountDialogOpen(false);
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  function openAddDiscount() {
    setEditingDiscount(null);
    setDiscountDialogOpen(true);
  }

  function openEditDiscount(discount: QuoteDiscountResponse) {
    setEditingDiscount(discount);
    setDiscountDialogOpen(true);
  }

  /* What a discount is called on screen: the seller's own description, else the promotion's name. */
  function discountName(discount: QuoteDiscountResponse): string {
    return discount.description ?? discount.promotion_name ?? '';
  }

  async function handleToggleDiscount(discount: QuoteDiscountResponse) {
    if (!quoteId) return;
    try {
      const updated = await updateDiscount(quoteId, discount.id, branchId, {
        suppressed_by_seller: !discount.suppressed_by_seller,
      });
      await onRefresh?.();
      onDiscountsChange?.(discounts.map((d) => (d.id === updated.id ? updated : d)));
      toast.success(
        t(
          updated.suppressed_by_seller
            ? 'detail.items.discounts.toast.suppressed'
            : 'detail.items.discounts.toast.restored',
          { name: discountName(updated) },
        ),
      );
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  async function handleDeleteDiscount(discount: QuoteDiscountResponse) {
    if (!quoteId) return;
    try {
      await deleteDiscount(quoteId, discount.id, branchId);
      onDiscountsChange?.(discounts.filter((d) => d.id !== discount.id));
      await onRefresh?.();
      toast.success(t('detail.items.discounts.toast.removed', { name: discountName(discount) }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  function openProductSearch(itemId?: string) {
    setEditingProductItemId(itemId ?? null);
    setSearchOpen(true);
  }

  async function handleQuantityBlur(itemId: string, currentQuantity: string) {
    if (!quoteId) return;
    const raw = editingQuantity[itemId];
    if (raw === undefined) return;
    const normalized = toQuantity(raw);
    if (normalized === currentQuantity) {
      setEditingQuantity((prev) => {
        const next = { ...prev };
        delete next[itemId];
        return next;
      });
      return;
    }

    try {
      const updated = await updateQuoteItem(quoteId, itemId, branchId, { quantity: normalized });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated', { name: updated.requested_description }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }

    setEditingQuantity((prev) => {
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  }

  async function handlePriceBlur(itemId: string, currentPrice: string) {
    if (!quoteId) return;
    const raw = editingPrice[itemId];
    if (raw === undefined) return;
    const normalized = raw.trim();
    // A field cleared to nothing is an edit the seller has not finished, not a price of zero.
    if (!normalized || Number(normalized) === Number(currentPrice)) {
      setEditingPrice((prev) => {
        const next = { ...prev };
        delete next[itemId];
        return next;
      });
      return;
    }

    try {
      const updated = await updateQuoteItem(quoteId, itemId, branchId, {
        unit_price_snapshot: normalized,
      });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated', { name: updated.requested_description }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }

    setEditingPrice((prev) => {
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  }

  /*
   * Confirmed rather than immediate. Re-adding the product is not an undo — it mints a new line and
   * re-runs pricing, so a quote that had been reviewed could come back with a different subtotal.
   * One click of friction is the honest price for a write the screen cannot take back.
   */
  async function handleDelete(item: QuoteItemResponse) {
    if (!quoteId) return;
    setDeleting(item.id);
    try {
      await deleteQuoteItem(quoteId, item.id, branchId);
      onItemsChange(items.filter((line) => line.id !== item.id));
      setPendingDelete(null);
      toast.success(t('detail.items.toast.deleted', { name: item.requested_description }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setDeleting(null);
    }
  }

  async function handleAddProduct(product: CatalogProduct) {
    if (!quoteId) return;
    try {
      const created = await addQuoteItem(quoteId, branchId, {
        product_id: product.id,
        requested_description: product.name,
        quantity: '1',
        unit: product.unit || null,
      });
      onItemsChange([...items, created]);
      toast.success(t('detail.items.toast.added', { name: created.requested_description }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  const editingItem = items.find((item) => item.id === editingProductItemId);

  async function handleModifyProduct(product: CatalogProduct) {
    if (!quoteId || !editingProductItemId) return;
    const item = items.find((i) => i.id === editingProductItemId);
    if (!item) return;
    try {
      const updated = await updateQuoteItem(quoteId, item.id, branchId, {
        product_id: product.id,
        requested_description: product.name,
        unit: product.unit || null,
      });
      // The write answers with the line alone; what matching offered for it still stands.
      const kept =
        updated.alternatives.length > 0 ? updated : { ...updated, alternatives: item.alternatives };
      onItemsChange(items.map((i) => (i.id === item.id ? kept : i)));
      toast.success(t('detail.items.toast.updated', { name: updated.requested_description }));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  if (items.length === 0) {
    return (
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle className="text-heading-5">{t('detail.items.title')}</CardTitle>
          {canEditProducts && (
            <Button type="button" variant="outline" size="sm" onClick={() => openProductSearch()}>
              <PlusIcon className="size-4" />
              {t('detail.items.add')}
            </Button>
          )}
        </CardHeader>
        <EmptyState
          icon={PackageOpenIcon}
          title={t('detail.items.empty')}
          description={canEditProducts ? t('detail.items.emptyHint') : undefined}
        />
        <ProductSearchDialog
          open={searchOpen}
          onOpenChange={setSearchOpen}
          onSelect={handleAddProduct}
        />
      </Card>
    );
  }

  return (
    <>
      <Card className="gap-y-0 overflow-hidden py-0">
        <CardHeader className="flex-row items-center justify-between py-4">
          <CardTitle className="text-heading-5">
            {t('detail.items.title')}
            <span className="ml-2 text-paragraph-sm text-foreground-muted">({items.length})</span>
          </CardTitle>
          {canEditProducts && (
            <Button type="button" variant="outline" size="sm" onClick={() => openProductSearch()}>
              <PlusIcon className="size-4" />
              {t('detail.items.add')}
            </Button>
          )}
        </CardHeader>

        <Table className="[&_th]:h-10 [&_td]:py-3">
          <TableHeader>
            {/*
             * One alignment rule across every table: copy reads from a common left edge, figures
             * line up on the right with tabular figures, and a column holding a single control is
             * centred under its own heading. Centring the copy too would cost the left edge the eye
             * follows down the column, which is the whole reason a table beats a list.
             */}
            <TableRow>
              <TableHead className="w-8 text-right">#</TableHead>
              <TableHead>{t('detail.items.columns.description')}</TableHead>
              <TableHead>{t('detail.items.columns.product')}</TableHead>
              {showConfidence && <TableHead>{t('detail.items.columns.confidence')}</TableHead>}
              <TableHead className="text-right">{t('detail.items.columns.quantity')}</TableHead>
              <TableHead>{t('detail.items.columns.unit')}</TableHead>
              {showPricing && (
                <TableHead className="text-right">{t('detail.items.columns.unitPrice')}</TableHead>
              )}
              {showPricing && (
                <TableHead className="text-right">{t('detail.items.columns.subtotal')}</TableHead>
              )}
              {(canEditProducts || canEditPrices) && (
                <TableHead className="w-20 text-center">
                  {t('detail.items.columns.actions')}
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item, index) => {
              const quantityValue = editingQuantity[item.id] ?? item.quantity;
              const priceValue = editingPrice[item.id] ?? item.unit_price_snapshot ?? '';
              const noMatch =
                !showConfidence && item.match_status === 'NO_MATCH' && !item.product_name;
              const confidence = confidenceBadge(item);

              return (
                <TableRow key={item.id}>
                  <TableCell className="text-right tabular-nums text-foreground-subtle">
                    {index + 1}
                  </TableCell>
                  <TableCell>
                    <span className="text-paragraph-sm-medium text-foreground">
                      {item.requested_description}
                    </span>
                  </TableCell>
                  <TableCell>
                    {item.product_name ? (
                      <div className="flex flex-col items-start gap-y-0.5">
                        {canEditProducts ? (
                          <button
                            type="button"
                            onClick={() => openProductSearch(item.id)}
                            className="group/product flex items-center gap-x-1 rounded-sm outline-none transition-colors duration-200 ease-out-soft hover:underline focus-visible:underline text-paragraph-sm text-foreground underline-offset-2"
                          >
                            {item.product_name}
                            <PencilIcon className="size-3 text-foreground-muted transition-colors duration-200 ease-out-soft group-hover/product:text-foreground group-focus-visible/product:text-foreground group-focus-visible/product:animate-focus-bump-soft" />
                          </button>
                        ) : (
                          <span className="text-paragraph-sm text-foreground">
                            {item.product_name}
                          </span>
                        )}
                        {item.product_code && (
                          <span className="text-paragraph-xs text-foreground-muted">
                            {item.product_code}
                          </span>
                        )}
                      </div>
                    ) : noMatch ? (
                      <div className="flex flex-col items-start gap-y-1">
                        <Badge tone="danger" size="sm">
                          {t('detail.items.matchStatus.NO_MATCH')}
                        </Badge>
                        {canEditProducts && (
                          <button
                            type="button"
                            onClick={() => openProductSearch(item.id)}
                            className={PRODUCT_TRIGGER}
                          >
                            {t('detail.items.chooseProduct')}
                            <PencilIcon className={PRODUCT_TRIGGER_ICON} />
                          </button>
                        )}
                      </div>
                    ) : item.product_id ? (
                      canEditProducts ? (
                        <button
                          type="button"
                          onClick={() => openProductSearch(item.id)}
                          className={PRODUCT_TRIGGER}
                        >
                          {item.product_id.slice(0, 8)}…
                          <PencilIcon className={PRODUCT_TRIGGER_ICON} />
                        </button>
                      ) : (
                        <span className="text-paragraph-xs text-foreground-muted">
                          {item.product_id.slice(0, 8)}…
                        </span>
                      )
                    ) : canEditProducts ? (
                      <button
                        type="button"
                        onClick={() => openProductSearch(item.id)}
                        className={PRODUCT_TRIGGER}
                      >
                        {t('detail.items.chooseProduct')}
                        <PencilIcon className={PRODUCT_TRIGGER_ICON} />
                      </button>
                    ) : (
                      <span className="text-foreground-subtle">—</span>
                    )}
                  </TableCell>
                  {showConfidence && (
                    <TableCell>
                      <Badge tone={confidence.tone} size="sm">
                        {t(`detail.confidence.${confidence.label}`)}
                      </Badge>
                    </TableCell>
                  )}
                  <TableCell className="text-right">
                    {canEditProducts ? (
                      <AmountInput
                        aria-label={t('detail.items.columns.quantity')}
                        maxDecimals={QUANTITY_DECIMALS}
                        value={quantityValue}
                        onChange={(next) =>
                          setEditingQuantity((prev) => ({ ...prev, [item.id]: next }))
                        }
                        onBlur={() => handleQuantityBlur(item.id, item.quantity)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            (event.target as HTMLInputElement).blur();
                          }
                        }}
                        containerClassName="w-24 ml-auto"
                        className="text-right tabular-nums"
                      />
                    ) : (
                      <span className="tabular-nums text-paragraph-sm">
                        {fmt.value(Number(item.quantity))}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-paragraph-sm">
                    {item.unit ?? item.product_unit ?? '—'}
                  </TableCell>
                  {showPricing && (
                    <TableCell className="text-right tabular-nums text-paragraph-sm">
                      {canEditPrices &&
                      (item.unit_price_snapshot != null || item.match_status === 'NO_MATCH') ? (
                        <AmountInput
                          aria-label={t('detail.items.columns.unitPrice')}
                          currency={ACCOUNT_CURRENCY}
                          prefix="$"
                          value={priceValue}
                          onChange={(next) =>
                            setEditingPrice((prev) => ({ ...prev, [item.id]: next }))
                          }
                          onBlur={() => handlePriceBlur(item.id, item.unit_price_snapshot ?? '0')}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                              (event.target as HTMLInputElement).blur();
                            }
                          }}
                          containerClassName="w-36 ml-auto"
                          className="text-right tabular-nums"
                        />
                      ) : item.unit_price_snapshot != null ? (
                        fmt.currency(item.unit_price_snapshot)
                      ) : (
                        <span className="text-foreground-subtle">—</span>
                      )}
                    </TableCell>
                  )}
                  {showPricing && (
                    <TableCell className="text-right tabular-nums text-paragraph-sm-medium">
                      {item.subtotal ? (
                        fmt.currency(item.subtotal)
                      ) : (
                        <span className="text-foreground-subtle">—</span>
                      )}
                    </TableCell>
                  )}
                  {(canEditProducts || canEditPrices) && (
                    <TableCell>
                      <div className="flex justify-center">
                        {/* Danger tone, like every other destructive row action — and unlike
                            archiving, removing a line is not something the screen can undo. */}
                        <RowActionButton
                          icon={TrashIcon}
                          tone="danger"
                          label={t('detail.items.delete')}
                          disabled={deleting !== null}
                          onClick={() => setPendingDelete(item)}
                        />
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>

        {showPricing && (
          <div className="border-t border-border px-4 py-3">
            <div className="flex flex-col gap-y-1.5">
              <div className="flex items-center justify-between text-paragraph-sm">
                <span className="text-foreground-muted">{t('detail.items.summary.subtotal')}</span>
                <span className="tabular-nums text-foreground">
                  {fmt.currency(String(itemsSubtotal))}
                </span>
              </div>

              {hasDiscounts && (
                <>
                  <span className="pt-1 text-paragraph-xs-semibold text-foreground-muted">
                    {t('detail.items.discounts.title')}
                  </span>
                  <div className="flex flex-col gap-y-1">
                    {discounts.map((discount) => (
                      <div
                        key={discount.id}
                        className="flex items-center justify-between gap-x-2 text-paragraph-xs"
                      >
                        <div className="flex min-w-0 items-center gap-x-1.5">
                          <MinusIcon className="size-3 shrink-0 text-foreground-muted" />
                          <span
                            className={
                              discount.suppressed_by_seller
                                ? 'text-foreground-muted line-through'
                                : 'text-foreground'
                            }
                          >
                            {discount.promotion_name ?? t('detail.items.discounts.adHoc')}
                          </span>
                          <Badge
                            tone={discount.suppressed_by_seller ? 'neutral' : 'success'}
                            size="sm"
                          >
                            {t(`detail.items.discounts.scope.${discount.scope}`)}
                          </Badge>
                          {discountRuleLabel(discount, fmt) && (
                            <Badge tone="neutral" size="sm">
                              {discountRuleLabel(discount, fmt)}
                            </Badge>
                          )}
                        </div>
                        <div className="flex shrink-0 items-center gap-x-2">
                          <span className="tabular-nums text-foreground-muted">
                            −{fmt.currency(discount.amount)}
                          </span>
                          {canEditDiscounts && (
                            <>
                              {discount.origin === 'MANUAL_SELLER' && (
                                <button
                                  type="button"
                                  className="text-foreground-muted transition-colors hover:text-foreground"
                                  onClick={() => openEditDiscount(discount)}
                                  aria-label={t('detail.items.discounts.edit')}
                                >
                                  <PencilIcon className="size-3" />
                                </button>
                              )}
                              {discount.origin !== 'MANUAL_SELLER' && (
                                <button
                                  type="button"
                                  className="text-foreground-muted transition-colors hover:text-foreground"
                                  onClick={() => handleToggleDiscount(discount)}
                                  aria-label={
                                    discount.suppressed_by_seller
                                      ? t('detail.items.discounts.restore')
                                      : t('detail.items.discounts.suppress')
                                  }
                                >
                                  {discount.suppressed_by_seller ? (
                                    <PlusIcon className="size-3" />
                                  ) : (
                                    <MinusIcon className="size-3" />
                                  )}
                                </button>
                              )}
                              {discount.origin === 'MANUAL_SELLER' && (
                                <button
                                  type="button"
                                  className="text-foreground-muted transition-colors hover:text-danger"
                                  onClick={() => handleDeleteDiscount(discount)}
                                  aria-label={t('detail.items.discounts.remove')}
                                >
                                  <TrashIcon className="size-3" />
                                </button>
                              )}
                            </>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              )}

              <div className="mt-1 flex items-center justify-between border-t border-border pt-2">
                <span className="text-paragraph-sm-semibold text-foreground">
                  {t('detail.items.total')}
                </span>
                <span className="text-paragraph-sm-semibold tabular-nums text-foreground">
                  {fmt.currency(String(Math.max(grandTotal, 0)))}
                </span>
              </div>
            </div>

            {canEditDiscounts && (
              <div className="mt-3 border-t border-border pt-3">
                <Button type="button" variant="outline" size="sm" onClick={openAddDiscount}>
                  <PlusIcon className="size-4" />
                  {t('detail.items.discounts.add')}
                </Button>
              </div>
            )}
          </div>
        )}
      </Card>

      <DiscountDialog
        open={discountDialogOpen}
        onOpenChange={setDiscountDialogOpen}
        initial={editingDiscount}
        items={items}
        onSave={handleDiscountSave}
      />

      <ProductSearchDialog
        open={searchOpen}
        onOpenChange={setSearchOpen}
        onSelect={editingProductItemId ? handleModifyProduct : handleAddProduct}
        title={
          editingItem
            ? t(
                editingItem.product_id
                  ? 'detail.items.columns.modifyProduct'
                  : 'detail.items.chooseProduct',
              )
            : undefined
        }
        suggestions={suggestionsFor(editingItem)}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(next) => !next && setPendingDelete(null)}
        entity={pendingDelete}
        pending={deleting !== null}
        title={t('detail.items.deleteConfirm.title')}
        description={(item) =>
          t('detail.items.deleteConfirm.description', { name: item.requested_description })
        }
        onConfirm={() => {
          if (pendingDelete) void handleDelete(pendingDelete);
        }}
        labels={{
          confirm: t('detail.items.deleteConfirm.confirm'),
          pending: t('detail.items.deleteConfirm.pending'),
          cancel: tCommon('actions.cancel'),
        }}
      />
    </>
  );
}
