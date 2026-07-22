import { getRequestConfig } from 'next-intl/server';

import { DEFAULT_LOCALE, TIME_ZONE } from '@/lib/i18n/locales';

/*
 * next-intl request config. Single-locale setup: no cookie/header negotiation and no `[locale]`
 * routing — the locale is pinned to Argentine Spanish and timestamps render in the Argentina
 * timezone. To add a language later, reintroduce negotiation here and add the entry in
 * `lib/i18n/locales.ts` + a `translations/<code>.json` file.
 */
export default getRequestConfig(async () => ({
  locale: DEFAULT_LOCALE,
  timeZone: TIME_ZONE,
  messages: (await import('../translations/es.json')).default,
}));
