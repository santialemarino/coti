'use client';

import { useMemo, useState } from 'react';
import { MinusIcon, PencilIcon, PercentIcon, PlusIcon, TrashIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardHeader,
  CardTitle,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import type { CatalogProduct } from '@/lib/api/catalog';
import type { QuoteDiscountResponse, QuoteItemResponse } from '@/lib/api/rfqs';
import { addQuoteItem, deleteQuoteItem, updateQuoteItem } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
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
}

function toQuantity(value: string): string {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return '1';
  return String(parsed);
}

function toPrice(value: string): string {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed < 0) return '0';
  return String(parsed);
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

export function RfqItemsTable({
  quoteId,
  quoteStatus,
  items,
  discounts,
  onItemsChange,
  onDiscountsChange,
}: RfqItemsTableProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const [searchOpen, setSearchOpen] = useState(false);
  const [editingQuantity, setEditingQuantity] = useState<Record<string, string>>({});
  const [editingPrice, setEditingPrice] = useState<Record<string, string>>({});
  const [discountsOpen, setDiscountsOpen] = useState(false);
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
      const updated = await updateQuoteItem(quoteId, itemId, { quantity: normalized });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated'));
    } catch {
      toast.error(t('detail.items.toast.error'));
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

    try {
      const updated = await updateQuoteItem(quoteId, itemId, {
        unit_price_snapshot: normalized,
      });
      onItemsChange(items.map((item) => (item.id === itemId ? updated : item)));
      toast.success(t('detail.items.toast.updated'));
    } catch {
      toast.error(t('detail.items.toast.error'));
    }

    setEditingPrice((prev) => {
      const next = { ...prev };
      delete next[itemId];
      return next;
    });
  }

  async function handleDelete(itemId: string) {
    if (!quoteId) return;
    try {
      await deleteQuoteItem(quoteId, itemId);
      onItemsChange(items.filter((item) => item.id !== itemId));
      toast.success(t('detail.items.toast.deleted'));
    } catch {
      toast.error(t('detail.items.toast.error'));
    }
  }

  async function handleAddProduct(product: CatalogProduct) {
    if (!quoteId) return;
    try {
      const created = await addQuoteItem(quoteId, {
        product_id: product.id,
        requested_description: product.name,
        quantity: '1',
        unit: product.unit || null,
      });
      onItemsChange([...items, created]);
      toast.success(t('detail.items.toast.added'));
    } catch {
      toast.error(t('detail.items.toast.error'));
    }
  }

  async function handleModifyProduct(product: CatalogProduct) {
    if (!quoteId || !editingProductItemId) return;
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
    } catch {
      toast.error(t('detail.items.toast.error'));
    } finally {
      setEditingProductItemId(null);
    }
  }

  function handleToggleDiscount(discountId: string) {
    if (!onDiscountsChange) return;
    onDiscountsChange(
      discounts.map((d) =>
        d.id === discountId ? { ...d, suppressed_by_seller: !d.suppressed_by_seller } : d,
      ),
    );
  }

  function handleDeleteDiscount(discountId: string) {
    if (!onDiscountsChange) return;
    onDiscountsChange(discounts.filter((d) => d.id !== discountId));
  }

  function handleAddAdHocDiscount() {
    if (!onDiscountsChange) return;
    onDiscountsChange([
      ...discounts,
      {
        id: crypto.randomUUID(),
        quote_version_id: '',
        promotion_id: null,
        promotion_name: null,
        condition_type: null,
        scope: 'TOTAL',
        origin: 'MANUAL_SELLER',
        amount: '0',
        suppressed_by_seller: false,
        created_at: new Date().toISOString(),
      },
    ]);
  }

  if (items.length === 0) {
    return (
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle className="text-heading-5">{t('detail.items.title')}</CardTitle>
          {canEditProducts && (
            <Button type="button" variant="outline" size="sm" onClick={() => setSearchOpen(true)}>
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
            <Button type="button" variant="outline" size="sm" onClick={() => setSearchOpen(true)}>
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
              const priceValue = editingPrice[item.id] ?? item.unit_price_snapshot ?? '';
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
                            onClick={() => {
                              setEditingProductItemId(item.id);
                              setSearchOpen(true);
                            }}
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
                          onClick={() => {
                            setEditingProductItemId(item.id);
                            setSearchOpen(true);
                          }}
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
                      {canEditPrices && item.unit_price_snapshot != null ? (
                        <Input
                          type="number"
                          inputMode="decimal"
                          min={0}
                          step={0.01}
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
                          containerClassName="w-28 mx-auto"
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

            {showPricing && (
              <TableRow className="bg-accent/50">
                <TableCell colSpan={showConfidence ? 5 : 4} />
                <TableCell
                  colSpan={2}
                  className="text-center text-paragraph-xs text-foreground-muted"
                >
                  {t('detail.items.total')}
                  {hasDiscounts && (
                    <span className="ml-1 text-foreground-subtle">
                      (−{fmt.currency(String(discountsTotal))})
                    </span>
                  )}
                </TableCell>
                <TableCell className="text-center text-paragraph-sm-semibold tabular-nums">
                  {fmt.currency(String(grandTotal))}
                </TableCell>
                {(canEditProducts || canEditPrices) && <TableCell />}
              </TableRow>
            )}
          </TableBody>
        </Table>

        {showPricing && (
          <div className="border-t border-border px-4 py-3">
            <Collapsible open={discountsOpen} onOpenChange={setDiscountsOpen}>
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="flex items-center gap-x-1.5 text-paragraph-xs font-medium text-foreground-muted hover:text-foreground transition-colors"
                >
                  <PercentIcon className="size-3" />
                  {t('detail.items.discounts.title')}
                  <span className="tabular-nums">
                    {hasDiscounts ? `(−${fmt.currency(String(discountsTotal))})` : ''}
                  </span>
                  <DropdownChevron open={discountsOpen} />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <div className="flex flex-col gap-y-1.5 pt-2">
                  {discounts.length === 0 ? (
                    <p className="text-paragraph-xs text-foreground-muted">
                      {t('detail.items.discounts.empty')}
                    </p>
                  ) : (
                    discounts.map((discount) => (
                      <div
                        key={discount.id}
                        className="flex items-center justify-between gap-x-2 text-paragraph-xs"
                      >
                        <div className="flex items-center gap-x-1.5 min-w-0">
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
                            {discount.scope === 'TOTAL'
                              ? 'Global'
                              : discount.scope === 'ITEM'
                                ? 'Por ítem'
                                : 'Combo'}
                          </Badge>
                        </div>
                        <div className="flex items-center gap-x-2 shrink-0">
                          <span className="tabular-nums text-foreground-muted">
                            −{fmt.currency(discount.amount)}
                          </span>
                          {canEditDiscounts && (
                            <>
                              <button
                                type="button"
                                className="text-foreground-muted hover:text-foreground transition-colors"
                                onClick={() => handleToggleDiscount(discount.id)}
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
                              <button
                                type="button"
                                className="text-foreground-muted hover:text-danger transition-colors"
                                onClick={() => handleDeleteDiscount(discount.id)}
                                aria-label={t('detail.items.discounts.remove')}
                              >
                                <TrashIcon className="size-3" />
                              </button>
                            </>
                          )}
                        </div>
                      </div>
                    ))
                  )}
                  {canEditDiscounts && (
                    <button
                      type="button"
                      onClick={handleAddAdHocDiscount}
                      className="mt-1 flex items-center gap-x-1.5 self-start text-paragraph-xs font-medium text-foreground-muted hover:text-foreground transition-colors"
                    >
                      <PlusIcon className="size-3" />
                      {t('detail.items.discounts.add')}
                    </button>
                  )}
                </div>
              </CollapsibleContent>
            </Collapsible>
          </div>
        )}
      </Card>

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
