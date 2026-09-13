import type { KeyboardEvent } from 'react';

import { numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

/*
 * The keystroke and sanitize rules below assume the active locale's decimal and group separators are
 * drawn from `.` and `,` — true for es-AR ("1.234,56") and en-US ("1,234.56"). The two getters are
 * locale-general because they read `Intl.formatToParts`; the rules that name a character are not.
 * A locale whose group separator is a space (fr-FR) or an apostrophe (de-CH) would need them
 * generalised first.
 */

// The locale's decimal separator — ',' for es-AR, '.' for en-US.
export function getDecimalSeparator(locale?: string): string {
  const parts = numberFormat(getLocaleTag(locale)).formatToParts(1.5);
  return parts.find((part) => part.type === 'decimal')?.value ?? ',';
}

// The locale's thousand-group separator — '.' for es-AR, ',' for en-US.
export function getGroupSeparator(locale?: string): string {
  const parts = numberFormat(getLocaleTag(locale)).formatToParts(1234.5);
  return parts.find((part) => part.type === 'group')?.value ?? '.';
}

// Groups a run of digits every three from the right. Input must already be digits only.
export function groupIntegerDigits(integer: string, groupSeparator: string): string {
  if (integer.length <= 3) return integer;
  return integer.replace(/\B(?=(\d{3})+(?!\d))/g, groupSeparator);
}

/*
 * Maps the caret across a regroup. Counts the significant characters — digits and the decimal
 * separator — to the left of the old caret and places the caret after that many in the new string.
 * Counting characters rather than indexes is the whole point: the group separators are exactly what
 * moved, so an index carried over drifts by one for every separator that appeared or vanished.
 */
export function mapCaretAfterRegroup(
  oldDisplay: string,
  oldCaret: number,
  newDisplay: string,
  decimalSeparator: string,
): number {
  const isSignificant = (character: string | undefined) =>
    character !== undefined &&
    ((character >= '0' && character <= '9') || character === decimalSeparator);

  let significant = 0;
  for (let index = 0; index < oldCaret && index < oldDisplay.length; index += 1) {
    if (isSignificant(oldDisplay[index])) significant += 1;
  }
  if (significant === 0) return 0;

  let seen = 0;
  for (let index = 0; index < newDisplay.length; index += 1) {
    if (isSignificant(newDisplay[index])) {
      seen += 1;
      if (seen === significant) return index + 1;
    }
  }
  return newDisplay.length;
}

/*
 * Locale display text → the canonical `.`-decimal string the API speaks (NUMERIC(14,2) travels as a
 * decimal string, never a float). `split/join` rather than `String.replace`, which only swaps the
 * first occurrence.
 */
export function normalizeAmountFromInput(input: string, locale?: string): string {
  if (!input) return '';
  const group = getGroupSeparator(locale);
  const decimal = getDecimalSeparator(locale);
  return input.split(group).join('').split(decimal).join('.');
}

/*
 * Canonical `.`-decimal → locale display text, grouped. A trailing `.` survives as a trailing
 * decimal separator so the caret can sit past it while the fraction is still being typed.
 */
export function formatAmountForInput(canonical: string, locale?: string): string {
  if (!canonical) return '';
  const group = getGroupSeparator(locale);
  const decimal = getDecimalSeparator(locale);
  const dotIndex = canonical.indexOf('.');
  const integer = dotIndex === -1 ? canonical : canonical.slice(0, dotIndex);
  const grouped = groupIntegerDigits(integer, group);
  if (dotIndex === -1) return grouped;
  return `${grouped}${decimal}${canonical.slice(dotIndex + 1)}`;
}

/* Truncates the fraction to `max` digits. `0` drops the separator; `undefined` means no limit. */
export function limitDecimalsInString(
  value: string,
  decimalSeparator: string,
  max: number | undefined,
): string {
  if (max === undefined) return value;
  const index = value.indexOf(decimalSeparator);
  if (index === -1) return value;
  const integer = value.slice(0, index);
  if (max === 0) return integer;
  return `${integer}${decimalSeparator}${value.slice(index + 1).slice(0, max)}`;
}

export type KeyRule = (event: KeyboardEvent<HTMLInputElement>) => void;

/* Runs each rule on every keystroke. `preventDefault` is idempotent, so overlap is harmless. */
export function composeKeyHandlers(...rules: KeyRule[]): KeyRule {
  return (event) => {
    for (const rule of rules) rule(event);
  };
}

// Amounts, quantities and counts in Coti are all non-negative, so neither sign ever gets through.
export function blockSignKeys(event: KeyboardEvent<HTMLInputElement>): void {
  if (event.key === '-' || event.key === '+') event.preventDefault();
}

// `<input type="number">` accepts scientific notation; none of these fields ever wants it.
export function blockScientificKeys(event: KeyboardEvent<HTMLInputElement>): void {
  if (event.key === 'e' || event.key === 'E') event.preventDefault();
}

/* Blocks the other locale's decimal separator, so a mixed-notation string can never be parsed. */
export function blockWrongLocaleDecimal(locale?: string): KeyRule {
  return (event) => {
    const wrong = getDecimalSeparator(locale) === '.' ? ',' : '.';
    if (event.key === wrong) event.preventDefault();
  };
}

/*
 * Blocks a second decimal separator — unless the selection covers the existing one, in which case
 * the keystroke replaces it rather than adding another.
 */
export function blockSecondDecimal(locale: string | undefined, currentValue: string): KeyRule {
  return (event) => {
    const decimal = getDecimalSeparator(locale);
    if (event.key !== decimal) return;
    const existing = currentValue.indexOf(decimal);
    if (existing === -1) return;
    const input = event.currentTarget;
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? input.value.length;
    if (start <= existing && existing < end) return;
    event.preventDefault();
  };
}

/* Blocks the separator outright for a zero-decimal field (a whole-currency total, a count). */
export function blockDecimalWhenIntegral(
  locale: string | undefined,
  maxDecimals: number | undefined,
): KeyRule {
  return (event) => {
    if (maxDecimals === 0 && event.key === getDecimalSeparator(locale)) event.preventDefault();
  };
}

// Integer mode: neither separator is ever valid.
export function blockAllSeparators(event: KeyboardEvent<HTMLInputElement>): void {
  if (event.key === '.' || event.key === ',') event.preventDefault();
}

/*
 * Keeps only digits and the locale's decimal separator, then collapses to a single separator
 * (keeping the first, keeping every digit). This is the safety net for the paths a keystroke rule
 * never sees — IME composition, autofill, a drag-and-drop of text — any of which could otherwise
 * leave a two-separator string that canonicalises to NaN.
 */
export function sanitizeDecimalChars(text: string, locale?: string): string {
  const decimal = getDecimalSeparator(locale);
  const disallowed = decimal === '.' ? /[^0-9.]/g : /[^0-9,]/g;
  const cleaned = text.replace(disallowed, '');
  const first = cleaned.indexOf(decimal);
  if (first === -1) return cleaned;
  return (
    cleaned.slice(0, first + 1) +
    cleaned
      .slice(first + 1)
      .split(decimal)
      .join('')
  );
}

/*
 * Sanitises pasted text by treating the LAST separator as the decimal and every earlier one as
 * grouping noise, so "1.234,56" and "1,234.56" both land on 1234.56 whatever the active locale.
 */
export function sanitizeDecimalPaste(text: string, locale?: string): string {
  const cleaned = text.replace(/[^0-9.,]/g, '');
  if (!cleaned) return '';

  const lastSeparator = Math.max(cleaned.lastIndexOf('.'), cleaned.lastIndexOf(','));
  if (lastSeparator === -1) return cleaned;

  const integer = cleaned.slice(0, lastSeparator).replace(/[.,]/g, '');
  const fraction = cleaned.slice(lastSeparator + 1).replace(/[.,]/g, '');
  // A lone separator canonicalises to "." and `Number('.')` is NaN, so it is dropped instead.
  if (!integer && !fraction) return '';
  return `${integer}${getDecimalSeparator(locale)}${fraction}`;
}

export function sanitizeIntegerPaste(text: string): string {
  return text.replace(/[^0-9]/g, '');
}

/*
 * Arrow-key stepping for a text-mode numeric field. The native stepper belongs to
 * `<input type="number">`, which these deliberately are not — it also binds the mouse wheel, so a
 * scroll past a focused field silently rewrites it. Returns the stepped canonical value, or null
 * when the key was not an arrow.
 */
export function stepCanonical(
  key: string,
  canonical: string,
  { step = 1, min = 0, maxDecimals }: { step?: number; min?: number; maxDecimals?: number } = {},
): string | null {
  if (key !== 'ArrowUp' && key !== 'ArrowDown') return null;
  const current = Number(canonical || '0');
  if (!Number.isFinite(current)) return null;
  const next = Math.max(min, current + (key === 'ArrowUp' ? step : -step));
  return maxDecimals === undefined ? String(next) : next.toFixed(maxDecimals);
}
