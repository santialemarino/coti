/*
 * Single source of truth for the app's UI locale. Coti ships one locale today —
 * Argentine Spanish. The registry shape is kept deliberately so adding a language
 * later is one entry here plus a matching `translations/<code>.json` file; no call
 * site threads a locale. (Renly's multi-locale negotiation is intentionally NOT
 * ported — see docs/internal/decisions.md.)
 */
export const LOCALES = [{ code: 'es', bcp47: 'es-AR', label: 'Español', dir: 'ltr' }] as const;

export type Locale = (typeof LOCALES)[number]['code'];
export type TextDirection = (typeof LOCALES)[number]['dir'];

// Locale used everywhere until a second language is added.
export const DEFAULT_LOCALE: Locale = 'es';

// IANA timezone all timestamps render in. Argentina is a single zone, so this is
// static rather than per-user; move it to a user preference if that changes.
export const TIME_ZONE = 'America/Argentina/Buenos_Aires';

// DEFAULT_LOCALE is always present in LOCALES, so this is never undefined.
const DEFAULT_ENTRY = LOCALES.find((l) => l.code === DEFAULT_LOCALE)!;

// Registry entry for a code, or the default-locale entry when the code is missing.
function localeEntry(locale?: string): (typeof LOCALES)[number] {
  return LOCALES.find((l) => l.code === locale) ?? DEFAULT_ENTRY;
}

// Resolves a locale code to a BCP47 tag (e.g. 'es-AR') for Intl APIs.
export function getLocaleTag(locale?: string): string {
  return localeEntry(locale).bcp47;
}

// Resolves a locale code to its text direction; readies the registry for a future RTL language.
export function getLocaleDirection(locale?: string): TextDirection {
  return localeEntry(locale).dir;
}
