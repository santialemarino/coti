'use client';

import { useMemo } from 'react';
import { useLocale, useTimeZone } from 'next-intl';

import { createFormatters } from '@/lib/i18n/create-formatters';

// Client hook returning locale-bound formatters (see `createFormatters`). Reads the active locale
// + timezone from next-intl so call sites never thread them. Memoised per (locale, timeZone).
export function useFormatters() {
  const locale = useLocale();
  const timeZone = useTimeZone();
  return useMemo(() => createFormatters(locale, timeZone), [locale, timeZone]);
}
