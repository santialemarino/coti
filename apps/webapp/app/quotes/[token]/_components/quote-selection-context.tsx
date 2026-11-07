'use client';

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';

import type { PublicQuoteAlternativeSelection } from '@/lib/api/public-quotes';

interface QuoteSelectionContextValue {
  selections: PublicQuoteAlternativeSelection[];
  select: (itemId: string, alternativeId?: string) => void;
  selectedAlternativeId: (itemId: string) => string | undefined;
  previewReady: boolean;
  setPreviewReady: (selections: PublicQuoteAlternativeSelection[], ready: boolean) => void;
}

const QuoteSelectionContext = createContext<QuoteSelectionContextValue | null>(null);

const emptySelectionContext: QuoteSelectionContextValue = {
  selections: [],
  select: () => undefined,
  selectedAlternativeId: () => undefined,
  previewReady: true,
  setPreviewReady: () => undefined,
};

export function QuoteSelectionProvider({ children }: { children: ReactNode }) {
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [previewedSelectionKey, setPreviewedSelectionKey] = useState<string | null>(null);
  const selections = useMemo(
    () => Object.entries(selected).map(([itemId, alternativeId]) => ({ itemId, alternativeId })),
    [selected],
  );
  const previewReady =
    selections.length === 0 || previewedSelectionKey === selectionKey(selections);
  const setPreviewReady = useCallback(
    (previewedSelections: PublicQuoteAlternativeSelection[], ready: boolean) => {
      setPreviewedSelectionKey(ready ? selectionKey(previewedSelections) : null);
    },
    [],
  );
  const value = useMemo<QuoteSelectionContextValue>(
    () => ({
      selections,
      previewReady,
      setPreviewReady,
      select: (itemId, alternativeId) => {
        setPreviewedSelectionKey(null);
        setSelected((current) => {
          if (!alternativeId) {
            const next = { ...current };
            delete next[itemId];
            return next;
          }
          return { ...current, [itemId]: alternativeId };
        });
      },
      selectedAlternativeId: (itemId) => selected[itemId],
    }),
    [previewReady, selected, selections, setPreviewReady],
  );
  return <QuoteSelectionContext.Provider value={value}>{children}</QuoteSelectionContext.Provider>;
}

export function useQuoteSelections(): QuoteSelectionContextValue {
  return useContext(QuoteSelectionContext) ?? emptySelectionContext;
}

function selectionKey(selections: PublicQuoteAlternativeSelection[]): string {
  return selections
    .map(({ itemId, alternativeId }) => `${itemId}:${alternativeId}`)
    .sort()
    .join('|');
}
