/*
 * Bounded cache of `Intl.*` instances. Constructing one resolves locale data and costs far more
 * than calling `.format()`, and dense surfaces format many cells per render. The keyspace is one
 * locale × a fixed set of option shapes, so no eviction is needed.
 */

const numberFormats = new Map<string, Intl.NumberFormat>();
const dateTimeFormats = new Map<string, Intl.DateTimeFormat>();
const listFormats = new Map<string, Intl.ListFormat>();

// `JSON.stringify` drops `undefined`-valued options, so an ambient-zone formatter shares a key
// with one built without the option — correct, they behave identically.
function cacheKey(locale: string, options: object): string {
  return `${locale}|${JSON.stringify(options)}`;
}

export function numberFormat(
  locale: string,
  options: Intl.NumberFormatOptions = {},
): Intl.NumberFormat {
  const key = cacheKey(locale, options);
  let formatter = numberFormats.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, options);
    numberFormats.set(key, formatter);
  }
  return formatter;
}

export function dateTimeFormat(
  locale: string,
  options: Intl.DateTimeFormatOptions = {},
): Intl.DateTimeFormat {
  const key = cacheKey(locale, options);
  let formatter = dateTimeFormats.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, options);
    dateTimeFormats.set(key, formatter);
  }
  return formatter;
}

export function listFormat(locale: string, options: Intl.ListFormatOptions = {}): Intl.ListFormat {
  const key = cacheKey(locale, options);
  let formatter = listFormats.get(key);
  if (!formatter) {
    formatter = new Intl.ListFormat(locale, options);
    listFormats.set(key, formatter);
  }
  return formatter;
}
