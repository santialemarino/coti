'use client';

import { useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  ScrollArea,
  SearchInput,
  Spinner,
} from '@repo/ui/components';
import { searchCatalog, type CatalogProduct } from '@/lib/api/catalog';

interface ProductSearchDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (product: CatalogProduct) => void;
  title?: string;
  /* The candidates matching already weighed for the line, listed first while they fit the query. */
  suggestions?: CatalogProduct[];
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
  suggestions = [],
}: ProductSearchDialogProps) {
  const t = useTranslations('rfqs.detail.items');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState<CatalogProduct[]>([]);
  const [loading, setLoading] = useState(false);
  const [wasOpen, setWasOpen] = useState(open);

  // Each opening starts a fresh search; clearing on close would empty the list mid-exit.
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setQuery('');
      setCatalog([]);
    }
  }

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    const timer = window.setTimeout(async () => {
      const results = await searchCatalog(query);
      if (cancelled) return;
      setCatalog(results);
      setLoading(false);
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [query, open]);

  const needle = folded(query.trim());
  const suggested = suggestions.filter(
    (product) => !needle || folded(`${product.name} ${product.code}`).includes(needle),
  );
  const suggestedIds = new Set(suggested.map((product) => product.id));
  // A search in flight hides the previous one's answer rather than show it under the new query.
  const results = [
    ...suggested,
    ...(loading ? [] : catalog.filter((product) => !suggestedIds.has(product.id))),
  ];

  function handleSelect(product: CatalogProduct) {
    onSelect(product);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title ?? t('searchProduct')}</DialogTitle>
        </DialogHeader>

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
          ) : results.length === 0 ? (
            <p className="py-8 text-center text-paragraph-sm text-foreground-muted">
              {query.trim() ? t('noResults') : t('typeToSearch')}
            </p>
          ) : (
            <ul className="flex flex-col gap-y-1">
              {results.map((product) => (
                <li key={product.id}>
                  <button
                    type="button"
                    className="flex w-full items-center justify-between p-3 gap-x-3 bg-card border border-border rounded-lg outline-none transition-[background-color,border-color,box-shadow] duration-150 ease-out-soft hover:bg-accent active:bg-accent-strong focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 text-left"
                    onClick={() => handleSelect(product)}
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
                          {t('suggested')}
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
      </DialogContent>
    </Dialog>
  );
}
