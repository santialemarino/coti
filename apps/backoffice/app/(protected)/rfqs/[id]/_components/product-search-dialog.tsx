'use client';

import { useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  SearchInput,
  Spinner,
} from '@repo/ui/components';
import { searchCatalog, type CatalogProduct } from '@/lib/api/catalog';

interface ProductSearchDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (product: CatalogProduct) => void;
  title?: string;
}

export function ProductSearchDialog({
  open,
  onOpenChange,
  onSelect,
  title,
}: ProductSearchDialogProps) {
  const t = useTranslations('rfqs.detail.items');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState<CatalogProduct[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) {
      setQuery('');
      setCatalog([]);
      return;
    }
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

  function handleSelect(product: CatalogProduct) {
    onSelect(product);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
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

        <div className="flex max-h-80 flex-col gap-y-2 overflow-y-auto">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Spinner size="sm" />
            </div>
          ) : catalog.length === 0 ? (
            <p className="py-8 text-center text-paragraph-sm text-foreground-muted">
              {query.trim() ? t('noResults') : t('typeToSearch')}
            </p>
          ) : (
            <ul className="flex flex-col gap-y-1">
              {catalog.map((product) => (
                <li key={product.id}>
                  <button
                    type="button"
                    className="flex w-full items-center justify-between gap-x-3 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:bg-accent"
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
                    <span className="shrink-0 text-paragraph-xs text-foreground-muted">
                      {t('select')}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
