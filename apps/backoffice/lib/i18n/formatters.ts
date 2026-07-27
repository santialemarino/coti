'use client';

import { useMemo } from 'react';
import { useLocale, useTimeZone } from 'next-intl';

import { createFormatters } from '@/lib/i18n/create-formatters';

// Locale-bound formatters for client components, memoised per (locale, timeZone).
export function useFormatters() {
  const locale = useLocale();
  const timeZone = useTimeZone();
  return useMemo(() => createFormatters(locale, timeZone), [locale, timeZone]);
}
