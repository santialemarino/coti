'use client';

import { useEffect, useMemo, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  ConfirmDialog,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  SearchInput,
  Spinner,
} from '@repo/ui/components';
import {
  listProductRecommendations,
  removeProductRecommendation,
  setProductRecommendation,
  type ProductRecommendation,
} from '@/app/(protected)/settings/catalog/actions';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { catalogCompatibility, searchCatalog, type CatalogProduct } from '@/lib/api/catalog';
import type { Product } from '@/lib/api/products';

type RecommendationType = 'ECONOMY' | 'PREMIUM';

interface PendingRecommendation {
  type: RecommendationType;
  product: CatalogProduct;
}

interface ProductRecommendationsDialogProps {
  product: Product | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ProductRecommendationsDialog({
  product,
  open,
  onOpenChange,
}: ProductRecommendationsDialogProps) {
  const t = useTranslations('products.recommendations');
  const message = useApiErrorMessage('products');
  const [recommendations, setRecommendations] = useState<ProductRecommendation[]>([]);
  const [loading, setLoading] = useState(false);
  const [pickerType, setPickerType] = useState<RecommendationType | null>(null);
  const [pending, setPending] = useState<PendingRecommendation | null>(null);
  const [removing, setRemoving] = useState<ProductRecommendation | null>(null);
  const [saving, startSaving] = useTransition();

  useEffect(() => {
    if (!open || !product) return;
    let cancelled = false;
    setLoading(true);
    void listProductRecommendations(product.id).then((result) => {
      if (cancelled) return;
      if (!result.ok) {
        toast.error(message(result.error));
        setRecommendations([]);
      } else {
        setRecommendations(result.items);
      }
      setLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [message, open, product]);

  function recommendationOf(type: RecommendationType) {
    return recommendations.find((recommendation) => recommendation.type === type) ?? null;
  }

  function confirmSelection() {
    if (!pending || !product) return;
    const next = pending;
    startSaving(async () => {
      const result = await setProductRecommendation(product.id, next.type, next.product.id);
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      const item: ProductRecommendation = {
        id: result.recommendationId ?? recommendationOf(next.type)?.id ?? next.product.id,
        productId: next.product.id,
        type: next.type,
        name: next.product.name,
        code: next.product.code || null,
        unit: next.product.unit || null,
      };
      setRecommendations((current) => [
        ...current.filter((recommendation) => recommendation.type !== next.type),
        item,
      ]);
      setPending(null);
      toast.success(t('saved', { name: next.product.name }));
    });
  }

  function confirmRemoval() {
    if (!removing || !product) return;
    const target = removing;
    startSaving(async () => {
      const result = await removeProductRecommendation(product.id, target.id);
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      setRecommendations((current) =>
        current.filter((recommendation) => recommendation.id !== target.id),
      );
      setRemoving(null);
      toast.success(t('removed', { name: target.name }));
    });
  }

  return (
    <>
      <Dialog open={open} onOpenChange={(next) => !saving && onOpenChange(next)}>
        <DialogContent className="sm:max-w-xl" closeOnClickOutside={!saving}>
          <DialogHeader>
            <DialogTitle>{t('title', { name: product?.name ?? '' })}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>

          {loading ? (
            <div className="flex justify-center py-10">
              <Spinner size="sm" />
            </div>
          ) : (
            <div className="flex flex-col gap-y-3">
              <RecommendationSlot
                type="ECONOMY"
                recommendation={recommendationOf('ECONOMY')}
                onSelect={() => setPickerType('ECONOMY')}
                onRemove={setRemoving}
                disabled={saving}
              />
              <RecommendationSlot
                type="PREMIUM"
                recommendation={recommendationOf('PREMIUM')}
                onSelect={() => setPickerType('PREMIUM')}
                onRemove={setRemoving}
                disabled={saving}
              />
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>
              {t('close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CatalogProductPicker
        open={pickerType !== null}
        onOpenChange={(next) => !next && setPickerType(null)}
        excludedProductId={product?.id ?? null}
        baseProduct={product}
        title={pickerType ? t(`picker.${pickerType}`) : ''}
        onSelect={(selected) => {
          if (!pickerType) return;
          setPending({ type: pickerType, product: selected });
          setPickerType(null);
        }}
      />

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(next) => !next && !saving && setPending(null)}
        entity={pending}
        title={t('replace.title')}
        description={(selection) =>
          t('replace.description', {
            type: t(`type.${selection.type}`),
            name: selection.product.name,
          })
        }
        onConfirm={confirmSelection}
        pending={saving}
        tone="default"
        labels={{
          confirm: t('replace.confirm'),
          pending: t('replace.pending'),
          cancel: t('cancel'),
        }}
      />

      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(next) => !next && !saving && setRemoving(null)}
        entity={removing}
        title={t('remove.title')}
        description={(recommendation) => t('remove.description', { name: recommendation.name })}
        onConfirm={confirmRemoval}
        pending={saving}
        labels={{ confirm: t('remove.confirm'), pending: t('remove.pending'), cancel: t('cancel') }}
      />
    </>
  );
}

function RecommendationSlot({
  type,
  recommendation,
  onSelect,
  onRemove,
  disabled,
}: {
  type: RecommendationType;
  recommendation: ProductRecommendation | null;
  onSelect: () => void;
  onRemove: (recommendation: ProductRecommendation) => void;
  disabled: boolean;
}) {
  const t = useTranslations('products.recommendations');
  const premium = type === 'PREMIUM';
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border border-border rounded-lg p-4">
      <div className="min-w-0 flex flex-col gap-y-1">
        <Badge
          tone={premium ? 'neutral' : 'success'}
          size="sm"
          className={
            premium
              ? 'w-fit border-recommendation-premium-border bg-recommendation-premium-subtle text-recommendation-premium-foreground'
              : 'w-fit'
          }
        >
          {t(`type.${type}`)}
        </Badge>
        {recommendation ? (
          <>
            <span className="text-paragraph-sm-medium text-foreground">{recommendation.name}</span>
            <span className="text-paragraph-xs text-foreground-muted">
              {[recommendation.code, recommendation.unit].filter(Boolean).join(' · ')}
            </span>
          </>
        ) : (
          <span className="text-paragraph-sm text-foreground-muted">{t('empty')}</span>
        )}
      </div>
      <div className="flex shrink-0 gap-x-2">
        {recommendation ? (
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() => onRemove(recommendation)}
          >
            {t('remove.action')}
          </Button>
        ) : null}
        <Button variant="outline" size="sm" disabled={disabled} onClick={onSelect}>
          {t(recommendation ? 'replace.action' : 'set')}
        </Button>
      </div>
    </div>
  );
}

function CatalogProductPicker({
  open,
  onOpenChange,
  excludedProductId,
  baseProduct,
  title,
  onSelect,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  excludedProductId: string | null;
  baseProduct: Product | null;
  title: string;
  onSelect: (product: CatalogProduct) => void;
}) {
  const t = useTranslations('products.recommendations');
  const [query, setQuery] = useState('');
  const [products, setProducts] = useState<CatalogProduct[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchError, setSearchError] = useState(false);
  const [searchAttempt, setSearchAttempt] = useState(0);
  const compatibility = useMemo(
    () =>
      catalogCompatibility(
        baseProduct
          ? { ...baseProduct, code: baseProduct.code ?? '', unit: baseProduct.unit ?? '' }
          : null,
      ),
    [baseProduct],
  );

  useEffect(() => {
    if (!open) return;
    setQuery('');
    setProducts([]);
  }, [open]);

  useEffect(() => {
    if (!open || !compatibility) return;
    let cancelled = false;
    setLoading(true);
    setSearchError(false);
    const timer = window.setTimeout(async () => {
      try {
        const results = await searchCatalog(query, compatibility);
        if (!cancelled) setProducts(results.filter((product) => product.id !== excludedProductId));
      } catch {
        if (!cancelled) setSearchError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [excludedProductId, open, query, compatibility, searchAttempt]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {compatibility ? t('picker.compatibility') : t('picker.missingClassification')}
          </DialogDescription>
        </DialogHeader>
        {compatibility && (
          <>
            <SearchInput
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onClear={() => setQuery('')}
              clearLabel={t('picker.clear')}
              placeholder={t('picker.placeholder')}
              containerClassName="w-full"
              autoFocus
            />
            <div className="flex max-h-80 flex-col overflow-y-auto gap-y-1">
              {loading ? (
                <div className="flex justify-center py-8">
                  <Spinner size="sm" />
                </div>
              ) : searchError ? (
                <div className="flex flex-col items-center gap-3 py-8 text-center">
                  <p role="alert" className="text-paragraph-sm text-foreground-muted">
                    {t('picker.searchError')}
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setSearchAttempt((current) => current + 1)}
                  >
                    {t('picker.retry')}
                  </Button>
                </div>
              ) : products.length === 0 ? (
                <p className="py-8 text-center text-paragraph-sm text-foreground-muted">
                  {query ? t('picker.noResults') : t('picker.noCompatibleProducts')}
                </p>
              ) : (
                products.map((candidate) => (
                  <button
                    key={candidate.id}
                    type="button"
                    className="flex w-full flex-col items-start border border-transparent rounded-lg p-3 text-left outline-none transition-[background-color,border-color,box-shadow] duration-150 ease-out-soft hover:border-border hover:bg-accent active:bg-accent-strong focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45"
                    onClick={() => onSelect(candidate)}
                  >
                    <span className="text-paragraph-sm-medium text-foreground">
                      {candidate.name}
                    </span>
                    <span className="text-paragraph-xs text-foreground-muted">
                      {[candidate.code, candidate.unit].filter(Boolean).join(' · ')}
                    </span>
                  </button>
                ))
              )}
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
