/*
 * Bounded cache of `Intl.*` instances, keyed by (localeTag, options). Constructing an
 * `Intl.NumberFormat` / `Intl.DateTimeFormat` / `Intl.ListFormat` resolves locale data and is
 * far more expensive than calling `.format()` on an existing one — and dense surfaces (quote
 * tables, catalog grids) format many cells per render. Every formatter routes construction
 * through here so the same (locale, options) pair reuses one instance. Output is byte-identical
 * to a fresh instance — this is purely a perf optimization.
 *
 * The keyspace is naturally bounded — one locale × a fixed set of option shapes — so a plain
 * `Map` never grows without bound, and the instances are stateless, so no eviction is needed.
 */

const numberFormats = new Map<string, Intl.NumberFormat>();
const dateTimeFormats = new Map<string, Intl.DateTimeFormat>();
const listFormats = new Map<string, Intl.ListFormat>();

// Cache key for a (locale, options) pair. `JSON.stringify` drops `undefined`-valued options
// (e.g. an unset `timeZone`), so an ambient-zone formatter caches under the same key as one
// built with the option omitted — correct, they behave identically.
function cacheKey(locale: string, options: object): string {
  return `${locale}|${JSON.stringify(options)}`;
}

// Returns a cached `Intl.NumberFormat` for the (locale, options) pair, constructing on first use.
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

// Returns a cached `Intl.DateTimeFormat` for the (locale, options) pair, constructing on first use.
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

// Returns a cached `Intl.ListFormat` for the (locale, options) pair, constructing on first use.
export function listFormat(locale: string, options: Intl.ListFormatOptions = {}): Intl.ListFormat {
  const key = cacheKey(locale, options);
  let formatter = listFormats.get(key);
  if (!formatter) {
    formatter = new Intl.ListFormat(locale, options);
    listFormats.set(key, formatter);
  }
  return formatter;
}
