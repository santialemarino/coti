'use client';

import { useMemo, useState } from 'react';
import { MinusIcon, PencilIcon, PlusIcon, TrashIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardHeader,
  CardTitle,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { CatalogProduct } from '@/lib/api/catalog';
import { errorCodeOf } from '@/lib/api/errors';
import type { CreateDiscountBody, QuoteDiscountResponse, QuoteItemResponse } from '@/lib/api/rfqs';
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
 *   QUOTED + CHANGE_REQUESTED prices (and discounts) editable
 *   SENT/ACCEPTED/REJECTED  read-only, prices shown
 */
const PRODUCT_EDIT_STATUSES = new Set(['DRAFT', 'CHANGE_REQUESTED']);
const PRICE_EDIT_STATUSES = new Set(['QUOTED', 'CHANGE_REQUESTED']);
const PRICED_STATUSES = new Set(['QUOTED', 'SENT', 'CHANGE_REQUESTED', 'ACCEPTED', 'REJECTED']);

interface RfqItemsTableProps {
  quoteId: string | null;
  quoteStatus: string | null;
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

function toPrice(value: string): string {
  const compact = value.trim().replace(/\s/g, '');
  const parts = compact.split('.');
  const normalized = compact.includes(',')
    ? compact.replace(/\./g, '').replace(',', '.')
    : parts.length === 2 && (parts[1]?.length ?? 0) <= 2
      ? compact
      : compact.replace(/\./g, '');
  if (!/^\d+(?:\.\d+)?$/.test(normalized)) return '0';

  const [integer, fraction] = normalized.split('.');
  const normalizedInteger = integer.replace(/^0+(?=\d)/, '');
  return fraction === undefined ? normalizedInteger : `${normalizedInteger}.${fraction}`;
}

function priceInputValue(value: string, fmt: ReturnType<typeof useFormatters>): string {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return fmt.value(parsed, { minDecimals: 2, maxDecimals: 2 });
}

function confidenceTone(score: string | null): 'success' | 'warning' | 'danger' | 'neutral' {
  if (!score) return 'neutral';
  const n = Number.parseFloat(score);
  if (n >= 0.8) return 'success';
  if (n >= 0.5) return 'warning';
  return 'danger';
}

function confidenceLabel(score: string | null): string {
  if (!score) return 'none';
  const n = Number.parseFloat(score);
  if (n >= 0.8) return 'high';
  if (n >= 0.5) return 'medium';
  return 'low';
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
  items,
  discounts,
  onItemsChange,
  onDiscountsChange,
  onRefresh,
}: RfqItemsTableProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const message = useApiErrorMessage('rfqs.detail.items');
  const { activeBranchId } = useRfqList();
  const [searchOpen, setSearchOpen] = useState(false);
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

  function requireActiveBranch(): boolean {
    if (activeBranchId) return true;
    toast.error(t('detail.items.toast.branchRequired'));
    return false;
  }

  async function handleDiscountSave(body: CreateDiscountBody) {
    if (!quoteId) return;
    if (!requireActiveBranch()) return;
    try {
      if (editingDiscount) {
        await updateDiscount(quoteId, editingDiscount.id, body);
        toast.success(t('detail.items.discounts.toast.updated'));
      } else {
        await addDiscount(quoteId, body);
        toast.success(t('detail.items.discounts.toast.added'));
      }
      await onRefresh?.();
      setDiscountDialogOpen(false);
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  function openAddDiscount() {
    if (!requireActiveBranch()) return;
    setEditingDiscount(null);
    setDiscountDialogOpen(true);
  }

  function openEditDiscount(discount: QuoteDiscountResponse) {
    if (!requireActiveBranch()) return;
    setEditingDiscount(discount);
    setDiscountDialogOpen(true);
  }

  async function handleToggleDiscount(discount: QuoteDiscountResponse) {
    if (!quoteId) return;
    if (!requireActiveBranch()) return;
    try {
      const updated = await updateDiscount(quoteId, discount.id, {
        suppressed_by_seller: !discount.suppressed_by_seller,
      });
      await onRefresh?.();
      onDiscountsChange?.(discounts.map((d) => (d.id === updated.id ? updated : d)));
      toast.success(
        updated.suppressed_by_seller
          ? t('detail.items.discounts.toast.suppressed')
          : t('detail.items.discounts.toast.restored'),
      );
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  async function handleDeleteDiscount(discount: QuoteDiscountResponse) {
    if (!quoteId) return;
    if (!requireActiveBranch()) return;
    try {
      await deleteDiscount(quoteId, discount.id);
      onDiscountsChange?.(discounts.filter((d) => d.id !== discount.id));
      await onRefresh?.();
      toast.success(t('detail.items.discounts.toast.removed'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  function openProductSearch(itemId?: string) {
    if (!requireActiveBranch()) return;
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
    if (!requireActiveBranch()) {
      setEditingQuantity((prev) => {
        const next = { ...prev };
        delete next[itemId];
        return next;
      });
      return;
    }

    try {
      const updated = await updateQuoteItem(quoteId, itemId, { quantity: normalized });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated'));
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
    const normalized = toPrice(raw);
    if (normalized === currentPrice) {
      setEditingPrice((prev) => {
        const next = { ...prev };
        delete next[itemId];
        return next;
      });
      return;
    }
    if (!requireActiveBranch()) {
      setEditingPrice((prev) => {
        const next = { ...prev };
        delete next[itemId];
        return next;
      });
      return;
    }

    try {
      const updated = await updateQuoteItem(quoteId, itemId, {
        unit_price_snapshot: normalized,
      });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }

    setEditingPrice((prev) => {
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  }

  async function handleDelete(itemId: string) {
    if (!quoteId) return;
    if (!requireActiveBranch()) return;
    try {
      await deleteQuoteItem(quoteId, itemId);
      onItemsChange(items.filter((item) => item.id !== itemId));
      toast.success(t('detail.items.toast.deleted'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  async function handleAddProduct(product: CatalogProduct) {
    if (!quoteId) return;
    if (!requireActiveBranch()) return;
    try {
      const created = await addQuoteItem(quoteId, {
        product_id: product.id,
        requested_description: product.name,
        quantity: '1',
        unit: product.unit || null,
      });
      onItemsChange([...items, created]);
      toast.success(t('detail.items.toast.added'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    }
  }

  async function handleModifyProduct(product: CatalogProduct) {
    if (!quoteId || !editingProductItemId) return;
    if (!requireActiveBranch()) return;
    const item = items.find((i) => i.id === editingProductItemId);
    if (!item) return;
    try {
      const updated = await updateQuoteItem(quoteId, item.id, {
        product_id: product.id,
        requested_description: product.name,
        unit: product.unit || null,
      });
      onItemsChange(items.map((i) => (i.id === item.id ? updated : i)));
      toast.success(t('detail.items.toast.updated'));
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setEditingProductItemId(null);
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
        <div className="px-6 py-8 text-center">
          <p className="text-paragraph-sm text-foreground-muted">{t('detail.items.empty')}</p>
        </div>
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
            <TableRow>
              <TableHead className="w-8 text-center">#</TableHead>
              <TableHead>{t('detail.items.columns.description')}</TableHead>
              <TableHead className="text-center">{t('detail.items.columns.product')}</TableHead>
              {showConfidence && (
                <TableHead className="text-center">
                  {t('detail.items.columns.confidence')}
                </TableHead>
              )}
              <TableHead className="text-center">{t('detail.items.columns.quantity')}</TableHead>
              <TableHead className="text-center">{t('detail.items.columns.unit')}</TableHead>
              {showPricing && (
                <TableHead className="text-center">{t('detail.items.columns.unitPrice')}</TableHead>
              )}
              {showPricing && (
                <TableHead className="text-center">{t('detail.items.columns.subtotal')}</TableHead>
              )}
              {(canEditProducts || canEditPrices) && <TableHead className="w-10" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item, index) => {
              const quantityValue = editingQuantity[item.id] ?? item.quantity;
              const priceValue =
                editingPrice[item.id] ??
                (item.unit_price_snapshot ? priceInputValue(item.unit_price_snapshot, fmt) : '');
              const noMatch =
                !showConfidence && item.match_status === 'NO_MATCH' && !item.product_name;

              return (
                <TableRow key={item.id}>
                  <TableCell className="text-center text-foreground-subtle">{index + 1}</TableCell>
                  <TableCell>
                    <span className="text-paragraph-sm-medium text-foreground">
                      {item.requested_description}
                    </span>
                  </TableCell>
                  <TableCell className="text-center">
                    {item.product_name ? (
                      <div className="flex flex-col items-center gap-y-0.5">
                        {canEditProducts ? (
                          <button
                            type="button"
                            onClick={() => openProductSearch(item.id)}
                            className="flex items-center gap-x-1 text-paragraph-sm text-foreground underline-offset-2 hover:underline"
                          >
                            {item.product_name}
                            <PencilIcon className="size-3 text-foreground-muted" />
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
                      <Badge tone="danger" size="sm">
                        {t('detail.items.matchStatus.NO_MATCH')}
                      </Badge>
                    ) : item.product_id ? (
                      canEditProducts ? (
                        <button
                          type="button"
                          onClick={() => openProductSearch(item.id)}
                          className="flex items-center gap-x-1 text-paragraph-xs text-foreground-muted underline-offset-2 hover:underline"
                        >
                          {item.product_id.slice(0, 8)}…
                          <PencilIcon className="size-3" />
                        </button>
                      ) : (
                        <span className="text-paragraph-xs text-foreground-muted">
                          {item.product_id.slice(0, 8)}…
                        </span>
                      )
                    ) : (
                      <span className="text-foreground-subtle">—</span>
                    )}
                  </TableCell>
                  {showConfidence && (
                    <TableCell className="text-center">
                      <Badge tone={confidenceTone(item.confidence_score)} size="sm">
                        {t(`detail.confidence.${confidenceLabel(item.confidence_score)}`)}
                      </Badge>
                    </TableCell>
                  )}
                  <TableCell className="text-center">
                    {canEditProducts ? (
                      <Input
                        type="number"
                        inputMode="decimal"
                        min={0}
                        step={0.01}
                        value={quantityValue}
                        onFocus={(event) => event.target.select()}
                        onChange={(event) =>
                          setEditingQuantity((prev) => ({
                            ...prev,
                            [item.id]: event.target.value,
                          }))
                        }
                        onBlur={() => handleQuantityBlur(item.id, item.quantity)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            (event.target as HTMLInputElement).blur();
                          }
                        }}
                        containerClassName="w-20 mx-auto"
                        className="text-center tabular-nums"
                      />
                    ) : (
                      <span className="tabular-nums text-paragraph-sm">
                        {fmt.value(Number(item.quantity))}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-center text-paragraph-sm">
                    {item.unit ?? item.product_unit ?? '—'}
                  </TableCell>
                  {showPricing && (
                    <TableCell className="text-center tabular-nums text-paragraph-sm">
                      {canEditPrices &&
                      (item.unit_price_snapshot != null || item.match_status === 'NO_MATCH') ? (
                        <Input
                          type="text"
                          inputMode="decimal"
                          prefix="$"
                          value={priceValue}
                          onFocus={(event) => event.target.select()}
                          onChange={(event) =>
                            setEditingPrice((prev) => ({
                              ...prev,
                              [item.id]: event.target.value,
                            }))
                          }
                          onBlur={() => handlePriceBlur(item.id, item.unit_price_snapshot ?? '0')}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                              (event.target as HTMLInputElement).blur();
                            }
                          }}
                          containerClassName="w-30 mx-auto"
                          className="text-center tabular-nums"
                        />
                      ) : item.unit_price_snapshot != null ? (
                        fmt.currency(item.unit_price_snapshot)
                      ) : (
                        <span className="text-foreground-subtle">—</span>
                      )}
                    </TableCell>
                  )}
                  {showPricing && (
                    <TableCell className="text-center tabular-nums text-paragraph-sm-medium">
                      {item.subtotal ? (
                        fmt.currency(item.subtotal)
                      ) : (
                        <span className="text-foreground-subtle">—</span>
                      )}
                    </TableCell>
                  )}
                  {(canEditProducts || canEditPrices) && (
                    <TableCell>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleDelete(item.id)}
                        aria-label={t('detail.items.delete')}
                      >
                        <TrashIcon className="size-4 text-foreground-muted" />
                      </Button>
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
                  <span className="pt-1 text-paragraph-xs font-semibold text-foreground-muted">
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
                <span className="text-paragraph-sm font-semibold text-foreground">
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
        onOpenChange={(open) => {
          if (!open) setEditingProductItemId(null);
          setSearchOpen(open);
        }}
        onSelect={editingProductItemId ? handleModifyProduct : handleAddProduct}
        title={editingProductItemId ? t('detail.items.columns.modifyProduct') : undefined}
      />
    </>
  );
}
