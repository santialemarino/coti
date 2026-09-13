'use client';

import { useEffect, useState, useTransition } from 'react';
import { PackageSearchIcon, PlusIcon, ShoppingCartIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Combobox,
  DialogFooter,
  EmptyState,
  Input,
  Label,
  MetaList,
  PendingButton,
  SearchInput,
  Skeleton,
} from '@repo/ui/components';
import { AmountInput } from '@/components/amount-input';
import { searchCatalog, type CatalogProduct } from '@/lib/api/catalog';
import { createRfq } from '@/lib/api/rfqs-client';
import { listSellers } from '@/lib/api/sellers';
import { useFormatters } from '@/lib/i18n/formatters';

interface RfqManualViewProps {
  onBack: () => void;
  onClose: () => void;
  onCreated: () => void;
  activeBranchId: string | null;
  /* Lets the dialog refuse a click outside once there is work a stray click would throw away. */
  onDirtyChange?: (dirty: boolean) => void;
}

/* How many placeholder rows stand in for the catalogue while it loads. */
const CATALOG_SKELETON_ROWS = 4;
// A line quantity is NUMERIC(14,2) on the wire, so entry is capped where storage is.
const QUANTITY_DECIMALS = 2;

interface LineItem {
  product: CatalogProduct;
  quantity: string;
}

/*
 * A line quantity is a measured figure — half a cubic metre of sand is a real order line — so it
 * keeps its decimals and only the empty and non-positive cases fall back to one unit.
 */
function toQuantity(value: string): string {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return '1';
  return value;
}

/*
 * The "cargar manualmente" step: the seller optionally names the client, searches the catalog and
 * builds the order line by line. Submitting calls POST /v1/rfqs.
 */
export function RfqManualView({
  onBack,
  onClose,
  onCreated,
  activeBranchId,
  onDirtyChange,
}: RfqManualViewProps) {
  const t = useTranslations('rfqs.create.manual');
  const tToast = useTranslations('rfqs.create.toast');
  const fmt = useFormatters();

  const [client, setClient] = useState('');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState<CatalogProduct[]>([]);
  const [loadingCatalog, setLoadingCatalog] = useState(true);
  const [items, setItems] = useState<LineItem[]>([]);
  const [seller, setSeller] = useState<string | null>(null);
  const [sellers, setSellers] = useState<Array<{ id: string; name: string }>>([]);
  const [loadingSellers, setLoadingSellers] = useState(true);
  const [submitting, startSubmit] = useTransition();

  /* Debounced search through the async seam, so a swap to the real endpoint changes nothing here. */
  useEffect(() => {
    let cancelled = false;
    setLoadingCatalog(true);
    const timer = window.setTimeout(async () => {
      const results = await searchCatalog(query);
      if (cancelled) return;
      setCatalog(results);
      setLoadingCatalog(false);
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [query]);

  /* Fetch the active branch's sellers so the new order can be assigned up front. */
  useEffect(() => {
    let cancelled = false;
    setLoadingSellers(true);
    (async () => {
      // A seller without an active branch cannot create a manual RFQ at all (the submit is
      // disabled), so there is nobody to offer on that path.
      const results = await listSellers(activeBranchId);
      if (cancelled) return;
      setSellers(results);
      setLoadingSellers(false);
    })();
    return () => {
      cancelled = true;
    };
  }, [activeBranchId]);

  function addProduct(product: CatalogProduct) {
    setItems((current) => {
      const existing = current.find((item) => item.product.id === product.id);
      if (existing) {
        return current.map((item) =>
          item.product.id === product.id
            ? { ...item, quantity: String(Number(item.quantity || 0) + 1) }
            : item,
        );
      }
      return [...current, { product, quantity: '1' }];
    });
  }

  function setQuantity(productId: string, value: string) {
    setItems((current) =>
      current.map((item) => (item.product.id === productId ? { ...item, quantity: value } : item)),
    );
  }

  function removeProduct(productId: string) {
    setItems((current) => current.filter((item) => item.product.id !== productId));
  }

  function onSubmit() {
    if (items.length === 0) return;
    startSubmit(async () => {
      try {
        await createRfq({
          client_label: client.trim() || null,
          seller_id: seller ?? null,
          items: items.map((item) => ({
            product_id: item.product.id,
            requested_description: item.product.name,
            quantity: item.quantity,
            unit: item.product.unit,
          })),
        });
        toast.success(tToast('created'));
        onCreated();
        onClose();
      } catch {
        toast.error(tToast('error'));
      }
    });
  }

  const disabled = items.length === 0 || !activeBranchId;
  // What a stray click outside the dialog would throw away.
  const dirty = items.length > 0 || client.trim() !== '' || seller !== null;

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      noValidate
      className="flex flex-col gap-y-5"
    >
      {activeBranchId ? null : <Callout tone="warning">{t('noBranch')}</Callout>}

      <div className="flex flex-col gap-y-4">
        <div className="flex flex-col gap-y-1">
          <Label htmlFor="rfq-manual-client">{t('clientLabel')}</Label>
          <Input
            id="rfq-manual-client"
            value={client}
            onChange={(event) => setClient(event.target.value)}
            placeholder={t('clientPlaceholder')}
            autoComplete="off"
          />
        </div>

        <div className="flex flex-col gap-y-1">
          <Label htmlFor="rfq-manual-seller">{t('sellerLabel')}</Label>
          <Combobox
            id="rfq-manual-seller"
            options={[
              { value: '', label: t('unassigned') },
              ...sellers.map((seller) => ({ value: seller.id, label: seller.name })),
            ]}
            value={seller}
            onValueChange={(value) => setSeller(value === '' ? null : value)}
            placeholder={t('sellerPlaceholder')}
            aria-label={t('sellerLabel')}
            className="min-w-64"
          />
          {activeBranchId && loadingSellers ? (
            <p className="text-paragraph-xs text-foreground-muted">{t('sellersLoading')}</p>
          ) : activeBranchId && sellers.length === 0 ? (
            <p className="text-paragraph-xs text-foreground-muted">{t('noSellers')}</p>
          ) : null}
        </div>

        <SearchInput
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onClear={() => setQuery('')}
          clearLabel={t('clearSearch')}
          placeholder={t('search')}
          containerClassName="w-full"
        />
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        {/*
         * Both columns are a fixed height whatever they hold. The catalogue arrives a moment after
         * the dialog does, and a column that grows into its result drags the dialog's own size with
         * it — which is the resize that reads as the dialog assembling itself on screen.
         */}
        <section className="flex min-w-0 flex-col gap-y-3">
          <p className="text-paragraph-xs-medium text-foreground-muted uppercase">
            {t('catalogLabel')}
          </p>
          <div className="h-80 overflow-y-auto pr-1">
            {loadingCatalog ? (
              <ul aria-busy="true" aria-label={t('loading')} className="flex flex-col gap-y-2">
                {Array.from({ length: CATALOG_SKELETON_ROWS }, (_, index) => (
                  <li key={index} className="flex h-[62px] items-center gap-x-3 p-3">
                    <div className="flex min-w-0 flex-1 flex-col gap-y-1.5">
                      <Skeleton className="h-3.5 w-2/3" />
                      <Skeleton className="h-2.5 w-1/3" />
                    </div>
                    <Skeleton className="h-8 w-20 shrink-0 rounded-lg" />
                  </li>
                ))}
              </ul>
            ) : catalog.length === 0 ? (
              <EmptyState icon={PackageSearchIcon} title={t('catalogEmpty')} />
            ) : (
              <ul className="flex flex-col gap-y-2">
                {catalog.map((product) => (
                  <li
                    key={product.id}
                    className="flex items-center justify-between gap-x-3 p-3 bg-card border border-border rounded-lg"
                  >
                    <div className="min-w-0">
                      <p className="truncate text-paragraph-sm-medium text-foreground">
                        {product.name}
                      </p>
                      <MetaList
                        className="text-paragraph-mini text-foreground-subtle"
                        items={[
                          product.code,
                          product.unit,
                          product.price ? fmt.currency(product.price) : null,
                        ]}
                      />
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => addProduct(product)}
                      className="shrink-0"
                    >
                      <PlusIcon aria-hidden="true" />
                      {t('add')}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>

        <section className="flex min-w-0 flex-col gap-y-3">
          <p className="text-paragraph-xs-medium text-foreground-muted uppercase">
            {t('itemsLabel', { count: items.length })}
          </p>
          <div className="h-80 overflow-y-auto pr-1">
            {items.length === 0 ? (
              <EmptyState icon={ShoppingCartIcon} title={t('itemsEmpty')} />
            ) : (
              <ul className="flex flex-col gap-y-2">
                {items.map(({ product, quantity }) => (
                  <li
                    key={product.id}
                    className="flex items-center gap-x-3 p-3 bg-card border border-border rounded-lg"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-paragraph-sm-medium text-foreground">
                        {product.name}
                      </p>
                      <p className="text-paragraph-mini text-foreground-subtle">
                        {product.price ? `${fmt.currency(product.price)} ${t('each')}` : t('each')}
                      </p>
                    </div>
                    <AmountInput
                      maxDecimals={QUANTITY_DECIMALS}
                      aria-label={t('quantityLabel', { name: product.name })}
                      value={quantity}
                      onChange={(next) => setQuantity(product.id, next)}
                      onBlur={() => setQuantity(product.id, toQuantity(quantity))}
                      containerClassName="w-24 flex-none"
                      className="text-right tabular-nums"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t('remove', { name: product.name })}
                      onClick={() => removeProduct(product.id)}
                    >
                      <XIcon aria-hidden="true" />
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" disabled={submitting} onClick={onBack}>
          {t('back')}
        </Button>
        <PendingButton
          type="submit"
          disabled={disabled}
          pending={submitting}
          pendingLabel={t('creating')}
        >
          {t('submit')}
        </PendingButton>
      </DialogFooter>
    </form>
  );
}
