/*
 * The app's UI locale. Coti ships one — Argentine Spanish — so there is nothing to negotiate.
 * The registry shape is kept so adding a language is one entry here plus a matching
 * `translations/<code>.json`, with no call site threading a locale.
 */
export const LOCALES = [{ code: 'es', bcp47: 'es-AR', label: 'Español', dir: 'ltr' }] as const;

export type Locale = (typeof LOCALES)[number]['code'];
export type TextDirection = (typeof LOCALES)[number]['dir'];

export const DEFAULT_LOCALE: Locale = 'es';

// Argentina is a single zone, so this is static; move it to a user preference if that changes.
export const TIME_ZONE = 'America/Argentina/Buenos_Aires';

// DEFAULT_LOCALE is always in LOCALES, so this is never undefined.
const DEFAULT_ENTRY = LOCALES.find((l) => l.code === DEFAULT_LOCALE)!;

function localeEntry(locale?: string): (typeof LOCALES)[number] {
  return LOCALES.find((l) => l.code === locale) ?? DEFAULT_ENTRY;
}

// Resolves a locale code to a BCP47 tag (e.g. 'es-AR') for Intl APIs.
export function getLocaleTag(locale?: string): string {
  return localeEntry(locale).bcp47;
}

// Text direction, readying the registry for a future RTL language.
export function getLocaleDirection(locale?: string): TextDirection {
  return localeEntry(locale).dir;
}
