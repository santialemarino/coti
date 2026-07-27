import { getRequestConfig } from 'next-intl/server';

import { DEFAULT_LOCALE, TIME_ZONE } from '@/lib/i18n/locales';

/*
 * next-intl request config. Single locale, so no negotiation and no `[locale]` routing. Adding a
 * language means reintroducing negotiation here plus an entry in `lib/i18n/locales.ts`.
 */
export default getRequestConfig(async () => ({
  locale: DEFAULT_LOCALE,
  timeZone: TIME_ZONE,
  messages: (await import('../translations/es.json')).default,
}));
