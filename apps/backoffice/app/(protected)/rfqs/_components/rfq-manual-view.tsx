'use client';

import { useEffect, useState, useTransition } from 'react';
import { AlertCircleIcon, PlusIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Button, DialogFooter, Input, PendingButton, SearchInput } from '@repo/ui/components';
import { searchCatalog, type CatalogProduct } from '@/lib/api/catalog';
import { createRfq } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

interface RfqManualViewProps {
  onBack: () => void;
  onClose: () => void;
  onCreated: () => void;
  activeBranchId: string | null;
}

interface LineItem {
  product: CatalogProduct;
  quantity: string;
}

function toQuantity(value: string): string {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return '1';
  return String(parsed);
}

/*
 * The "cargar manualmente" step: the seller optionally names the client, searches the catalog and
 * builds the order line by line. Submitting calls POST /v1/rfqs.
 */
export function RfqManualView({ onBack, onClose, onCreated, activeBranchId }: RfqManualViewProps) {
  const t = useTranslations('rfqs.create.manual');
  const tToast = useTranslations('rfqs.create.toast');
  const fmt = useFormatters();

  const [client, setClient] = useState('');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState<CatalogProduct[]>([]);
  const [loadingCatalog, setLoadingCatalog] = useState(true);
  const [items, setItems] = useState<LineItem[]>([]);
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

  function addProduct(product: CatalogProduct) {
    setItems((current) => {
      const existing = current.find((item) => item.product.id === product.id);
      if (existing) {
        return current.map((item) =>
          item.product.id === product.id
            ? { ...item, quantity: toQuantity(String(Number(item.quantity) + 1)) }
            : item,
        );
      }
      return [...current, { product, quantity: '1' }];
    });
  }

  function setQuantity(productId: string, value: string) {
    setItems((current) =>
      current.map((item) =>
        item.product.id === productId ? { ...item, quantity: toQuantity(value) } : item,
      ),
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

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      noValidate
      className="flex flex-col gap-y-5"
    >
      {activeBranchId ? null : (
        <div className="flex items-center gap-x-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-paragraph-sm text-amber-800">
          <AlertCircleIcon aria-hidden="true" className="shrink-0 size-4" />
          {t('noBranch')}
        </div>
      )}

      <div className="flex flex-col gap-y-4">
        <div className="flex flex-col gap-y-1">
          <label htmlFor="rfq-manual-client" className="text-paragraph-sm-medium">
            {t('clientLabel')}
          </label>
          <Input
            id="rfq-manual-client"
            value={client}
            onChange={(event) => setClient(event.target.value)}
            placeholder={t('clientPlaceholder')}
            autoComplete="off"
          />
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
        <section className="flex min-w-0 flex-col gap-y-3">
          <p className="text-paragraph-xs-semibold uppercase tracking-wide text-foreground-muted">
            {t('catalogLabel')}
          </p>
          {loadingCatalog ? (
            <p className="text-paragraph-sm text-foreground-muted">{t('loading')}</p>
          ) : catalog.length === 0 ? (
            <p className="text-paragraph-sm text-foreground-muted">{t('catalogEmpty')}</p>
          ) : (
            <ul className="flex max-h-80 flex-col gap-y-2 overflow-y-auto pr-1">
              {catalog.map((product) => (
                <li
                  key={product.id}
                  className="flex items-center justify-between gap-x-3 rounded-lg border border-border bg-card p-3"
                >
                  <div className="min-w-0">
                    <p className="truncate text-paragraph-sm-medium text-foreground">
                      {product.name}
                    </p>
                    <p className="truncate text-paragraph-mini text-foreground-subtle">
                      {product.code} · {product.unit}
                      {product.price ? <> · {fmt.currency(product.price)}</> : null}
                    </p>
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
        </section>

        <section className="flex min-w-0 flex-col gap-y-3">
          <p className="text-paragraph-xs-semibold uppercase tracking-wide text-foreground-muted">
            {t('itemsLabel', { count: items.length })}
          </p>
          {items.length === 0 ? (
            <p className="text-paragraph-sm text-foreground-muted">{t('itemsEmpty')}</p>
          ) : (
            <ul className="flex max-h-80 flex-col gap-y-2 overflow-y-auto pr-1">
              {items.map(({ product, quantity }) => (
                <li
                  key={product.id}
                  className="flex items-center gap-x-3 rounded-lg border border-border bg-card p-3"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-paragraph-sm-medium text-foreground">
                      {product.name}
                    </p>
                    <p className="text-paragraph-mini text-foreground-subtle">
                      {product.price ? (
                        <>
                          {fmt.currency(product.price)} {t('each')}
                        </>
                      ) : (
                        t('each')
                      )}
                    </p>
                  </div>
                  <Input
                    type="number"
                    inputMode="numeric"
                    min={1}
                    aria-label={t('quantityLabel', { name: product.name })}
                    value={quantity}
                    onFocus={(event) => event.target.select()}
                    onChange={(event) => setQuantity(product.id, event.target.value)}
                    onBlur={(event) => {
                      const normalized = toQuantity(event.target.value);
                      if (normalized !== event.target.value) {
                        setQuantity(product.id, normalized);
                      }
                    }}
                    containerClassName="w-20 flex-none"
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
