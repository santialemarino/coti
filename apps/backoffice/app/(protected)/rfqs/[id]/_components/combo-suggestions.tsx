'use client';

import { useEffect, useState } from 'react';
import { PackagePlusIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  PendingButton,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type { QuoteItemResponse } from '@/lib/api/rfqs';
import {
  addComboItems,
  fetchComboSuggestions,
  type ComboSuggestion,
  type ComboSuggestionItem,
} from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { shortUnit } from '@/lib/utils/units';

interface ComboSuggestionsProps {
  quoteId: string;
  branchId: string;
  /* The quote's lines; a change in which products it holds asks for the suggestions again. */
  items: QuoteItemResponse[];
  /* Reloads the quote after lines were added, so totals and promotions come back together. */
  onAdded: () => Promise<void>;
}

/*
 * The catalog combos a priced quote holds part of. Adding one turns its missing products into
 * ordinary priced lines; a bundle discount, when the supplier set one, is a promotion the engine
 * applies on its own. Nothing renders while there is nothing to suggest.
 */
export function ComboSuggestions({ quoteId, branchId, items, onAdded }: ComboSuggestionsProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.combos');
  const message = useApiErrorMessage('rfqs.detail.combos');
  const [suggestions, setSuggestions] = useState<ComboSuggestion[]>([]);
  const [adding, setAdding] = useState<string | null>(null);
  const productKey = [
    ...new Set(items.flatMap((item) => (item.product_id ? [item.product_id] : []))),
  ]
    .sort()
    .join(',');

  useEffect(() => {
    let cancelled = false;
    fetchComboSuggestions(quoteId, branchId)
      .then((found) => {
        if (!cancelled) setSuggestions(found);
      })
      // A suggestion is an extra: failing to load one must not get in the way of the quote.
      .catch(() => {
        if (!cancelled) setSuggestions([]);
      });
    return () => {
      cancelled = true;
    };
  }, [quoteId, branchId, productKey]);

  async function onAdd(suggestion: ComboSuggestion) {
    if (adding) return;
    setAdding(suggestion.comboId);
    try {
      const added = await addComboItems(quoteId, suggestion.comboId, branchId);
      toast.success(t('added', { count: added.length, name: suggestion.name }));
      await onAdded();
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setAdding(null);
    }
  }

  function line(item: ComboSuggestionItem) {
    const unit = item.productUnit ? ` ${shortUnit(item.productUnit)}` : '';
    return t('line', { quantity: fmt.value(Number(item.quantity)) + unit, name: item.productName });
  }

  if (suggestions.length === 0) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="flex flex-col gap-y-4">
          {suggestions.map((suggestion) => (
            <li
              key={suggestion.comboId}
              className="flex flex-col p-4 gap-y-3 border border-border rounded-lg sm:flex-row sm:items-start sm:justify-between sm:gap-x-4"
            >
              <div className="flex flex-col min-w-0 gap-y-2">
                <p className="text-paragraph-sm-medium text-foreground">{suggestion.name}</p>
                {suggestion.description ? (
                  <p className="text-paragraph-xs text-foreground-muted">
                    {suggestion.description}
                  </p>
                ) : null}
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-paragraph-xs text-foreground-muted">{t('missing')}</span>
                  {suggestion.missing.map((item) => (
                    <Badge key={item.productId} tone="brand" size="sm">
                      {line(item)}
                    </Badge>
                  ))}
                </div>
                <p className="text-paragraph-xs text-foreground-subtle">
                  {t('present', {
                    names: fmt.list(suggestion.present.map((item) => item.productName)),
                  })}
                </p>
              </div>
              <PendingButton
                variant="outline"
                size="sm"
                className="shrink-0"
                pending={adding === suggestion.comboId}
                pendingLabel={t('adding')}
                disabled={adding !== null}
                onClick={() => void onAdd(suggestion)}
              >
                <PackagePlusIcon aria-hidden="true" />
                {t('add', { count: suggestion.missing.length })}
              </PendingButton>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
