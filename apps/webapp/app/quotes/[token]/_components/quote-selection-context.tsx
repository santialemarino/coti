'use client';

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

import type { PublicQuoteAlternativeSelection } from '@/lib/api/public-quotes';

interface QuoteSelectionContextValue {
  selections: PublicQuoteAlternativeSelection[];
  select: (itemId: string, alternativeId?: string) => void;
  selectedAlternativeId: (itemId: string) => string | undefined;
}

const QuoteSelectionContext = createContext<QuoteSelectionContextValue | null>(null);

const emptySelectionContext: QuoteSelectionContextValue = {
  selections: [],
  select: () => undefined,
  selectedAlternativeId: () => undefined,
};

export function QuoteSelectionProvider({ children }: { children: ReactNode }) {
  const [selected, setSelected] = useState<Record<string, string>>({});
  const value = useMemo<QuoteSelectionContextValue>(
    () => ({
      selections: Object.entries(selected).map(([itemId, alternativeId]) => ({
        itemId,
        alternativeId,
      })),
      select: (itemId, alternativeId) => {
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
    [selected],
  );
  return <QuoteSelectionContext.Provider value={value}>{children}</QuoteSelectionContext.Provider>;
}

export function useQuoteSelections(): QuoteSelectionContextValue {
  return useContext(QuoteSelectionContext) ?? emptySelectionContext;
}
