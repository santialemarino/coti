'use client';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  ScrollArea,
  SearchInput,
  Spinner,
} from '@repo/ui/components';
import {
  catalogCompatibility,
  getCatalogProduct,
  searchCatalog,
  type CatalogProduct,
} from '@/lib/api/catalog';
import type { RecommendationType } from '@/lib/api/rfqs';

export type ProductSuggestion = CatalogProduct & {
  recommendationType?: RecommendationType | null;
};

interface ProductSearchDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (product: CatalogProduct) => void | boolean | Promise<void | boolean>;
  title?: string;
  footer?: ReactNode;
  /* The candidates matching already weighed for the line, listed first while they fit the query. */
  suggestions?: ProductSuggestion[];
  excludedProductIds?: string[];
  compatibilityProductId?: string | null;
}

function folded(text: string): string {
  return text
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase();
}

export function ProductSearchDialog({
  open,
  onOpenChange,
  onSelect,
  title,
  footer,
  suggestions = [],
  excludedProductIds = [],
  compatibilityProductId,
}: ProductSearchDialogProps) {
  const t = useTranslations('rfqs.detail.items');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState<ProductSuggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchError, setSearchError] = useState(false);
  const [searchAttempt, setSearchAttempt] = useState(0);
  const [baseAttempt, setBaseAttempt] = useState(0);
  const [baseResult, setBaseResult] = useState<{
    id: string;
    product: CatalogProduct | null;
  } | null>(null);
  const [wasOpen, setWasOpen] = useState(open);
  const [selecting, setSelecting] = useState(false);
  const selectionInFlight = useRef(false);

  // Each opening starts a fresh search; clearing on close would empty the list mid-exit.
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setQuery('');
      setCatalog([]);
      setBaseResult(null);
    }
  }

  useEffect(() => {
    if (!open || !compatibilityProductId) return;
    let cancelled = false;
    void getCatalogProduct(compatibilityProductId).then((product) => {
      if (!cancelled) setBaseResult({ id: compatibilityProductId, product });
    });
    return () => {
      cancelled = true;
    };
  }, [open, compatibilityProductId, baseAttempt]);

  const baseLoading = !!compatibilityProductId && baseResult?.id !== compatibilityProductId;
  const baseProduct = !baseLoading ? baseResult?.product : undefined;
  const compatibility = useMemo(
    () => (compatibilityProductId ? catalogCompatibility(baseProduct ?? null) : null),
    [compatibilityProductId, baseProduct],
  );

  useEffect(() => {
    if (!open || (compatibilityProductId && !compatibility)) return;
    let cancelled = false;
    setLoading(true);
    setSearchError(false);
    const timer = window.setTimeout(async () => {
      try {
        const results = await searchCatalog(query, compatibility ?? undefined);
        if (!cancelled) setCatalog(results);
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
  }, [query, open, compatibilityProductId, compatibility, searchAttempt]);

  const needle = folded(query.trim());
  const excluded = new Set(excludedProductIds);
  const suggested = suggestions.filter(
    (product) =>
      !excluded.has(product.id) &&
      (!needle || folded(`${product.name} ${product.code}`).includes(needle)),
  );
  const suggestedIds = new Set(suggested.map((product) => product.id));
  // A search in flight hides the previous one's answer rather than show it under the new query.
  const results = [
    ...suggested,
    ...(loading
      ? []
      : catalog.filter((product) => !suggestedIds.has(product.id) && !excluded.has(product.id))),
  ];

  function suggestedLabel(product: ProductSuggestion): string {
    switch (product.recommendationType) {
      case 'ECONOMY':
        return t('recommendationType.ECONOMY');
      case 'PREMIUM':
        return t('recommendationType.PREMIUM');
      case 'EQUIVALENT':
        return t('recommendationType.EQUIVALENT');
      default:
        return t('suggested');
    }
  }

  async function handleSelect(product: CatalogProduct) {
    if (selectionInFlight.current) return;
    selectionInFlight.current = true;
    setSelecting(true);
    try {
      const selected = await onSelect(product);
      if (selected !== false) onOpenChange(false);
    } finally {
      selectionInFlight.current = false;
      setSelecting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" closeOnClickOutside={!selecting}>
        <DialogHeader>
          <DialogTitle>{title ?? t('searchProduct')}</DialogTitle>
          {compatibilityProductId && !baseLoading && (
            <DialogDescription>
              {baseProduct
                ? compatibility
                  ? t('alternatives.picker.compatibility')
                  : t('alternatives.picker.missingClassification')
                : t('alternatives.picker.loadError')}
            </DialogDescription>
          )}
        </DialogHeader>

        {baseLoading ? (
          <div className="flex justify-center py-8">
            <Spinner size="sm" />
          </div>
        ) : compatibilityProductId && !baseProduct ? (
          <div className="flex justify-center py-4">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setBaseResult(null);
                setBaseAttempt((current) => current + 1);
              }}
            >
              {t('alternatives.picker.retry')}
            </Button>
          </div>
        ) : (
          (!compatibilityProductId || compatibility) && (
            <>
              <SearchInput
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onClear={() => setQuery('')}
                clearLabel="Limpiar"
                placeholder={t('searchProductPlaceholder')}
                containerClassName="w-full"
                autoFocus
              />

              {/* A fixed height: the results land after the dialog opens and must not grow it. */}
              <ScrollArea className="flex h-80 flex-col gap-y-2">
                {loading && suggested.length === 0 ? (
                  <div className="flex items-center justify-center py-8">
                    <Spinner size="sm" />
                  </div>
                ) : searchError ? (
                  <div className="flex flex-col items-center gap-3 py-8 text-center">
                    <p role="alert" className="text-paragraph-sm text-foreground-muted">
                      {t('alternatives.picker.searchError')}
                    </p>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setSearchAttempt((current) => current + 1)}
                    >
                      {t('alternatives.picker.retry')}
                    </Button>
                  </div>
                ) : results.length === 0 ? (
                  <p className="py-8 text-center text-paragraph-sm text-foreground-muted">
                    {query.trim()
                      ? t('noResults')
                      : compatibilityProductId
                        ? t('alternatives.picker.noCompatibleProducts')
                        : t('typeToSearch')}
                  </p>
                ) : (
                  <ul className="flex flex-col gap-y-1">
                    {results.map((product) => (
                      <li key={product.id}>
                        <button
                          type="button"
                          disabled={selecting}
                          className="flex w-full items-center justify-between p-3 gap-x-3 bg-card border border-border rounded-lg outline-none transition-[background-color,border-color,box-shadow] duration-150 ease-out-soft hover:bg-accent active:bg-accent-strong focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 disabled:pointer-events-none disabled:opacity-50 text-left"
                          onClick={() => void handleSelect(product)}
                        >
                          <div className="min-w-0">
                            <p className="truncate text-paragraph-sm-medium text-foreground">
                              {product.name}
                            </p>
                            <p className="truncate text-paragraph-xs text-foreground-muted">
                              {product.code} · {product.unit}
                            </p>
                          </div>
                          <span className="flex shrink-0 items-center gap-x-2 text-paragraph-xs text-foreground-muted">
                            {suggestedIds.has(product.id) && (
                              <Badge tone="brand" size="sm">
                                {suggestedLabel(product)}
                              </Badge>
                            )}
                            {t('select')}
                          </span>
                        </button>
                      </li>
                    ))}
                    {loading && (
                      <li className="flex items-center justify-center py-3">
                        <Spinner size="sm" />
                      </li>
                    )}
                  </ul>
                )}
              </ScrollArea>

              {footer ? <div className="border-t border-border pt-3">{footer}</div> : null}
            </>
          )
        )}
      </DialogContent>
    </Dialog>
  );
}
