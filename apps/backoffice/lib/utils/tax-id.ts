import { CUIT_DIGITS } from '@/lib/utils/cuit';

export type TaxIdKind = 'cuit' | 'dni' | 'cuit-or-dni';

const DNI_MIN_DIGITS = 7;
const DNI_MAX_DIGITS = 8;
const CUIT_PREFIX_DIGITS = 2;
const CUIT_BODY_END = 10;
const DNI_GROUP = 3;
const CUIT_SEPARATOR = '-';
const DNI_SEPARATOR = '.';

export const TAX_ID_MAX_DIGITS: Record<TaxIdKind, number> = {
  cuit: CUIT_DIGITS,
  dni: DNI_MAX_DIGITS,
  'cuit-or-dni': CUIT_DIGITS,
};

export function taxIdDigits(raw: string): string {
  return raw.replace(/\D/g, '');
}

// Whether the digits are as many as the kind holds: eleven for a CUIT, seven or eight for a DNI.
export function hasTaxIdLength(digits: string, kind: TaxIdKind): boolean {
  const cuit = digits.length === CUIT_DIGITS;
  const dni = digits.length >= DNI_MIN_DIGITS && digits.length <= DNI_MAX_DIGITS;
  if (kind === 'cuit') return cuit;
  if (kind === 'dni') return dni;
  return cuit || dni;
}

function cuitShape(digits: string): string {
  if (digits.length <= CUIT_PREFIX_DIGITS) return digits;
  const prefix = digits.slice(0, CUIT_PREFIX_DIGITS);
  const body = digits.slice(CUIT_PREFIX_DIGITS, CUIT_BODY_END);
  const check = digits.slice(CUIT_BODY_END);
  return [prefix, body, check].filter(Boolean).join(CUIT_SEPARATOR);
}

// Grouped in thousands from the right, so a DNI reads the same while typed as once complete.
function dniShape(digits: string): string {
  return digits.replace(new RegExp(`\\B(?=(\\d{${DNI_GROUP}})+$)`, 'g'), DNI_SEPARATOR);
}

/*
 * The display for the digits typed so far; a separator appears once a digit follows it. A
 * `cuit-or-dni` reads as a DNI up to eight digits, and as a CUIT from the ninth, since no DNI is longer.
 */
export function formatTaxId(digits: string, kind: TaxIdKind): string {
  if (kind === 'dni') return dniShape(digits);
  if (kind === 'cuit-or-dni' && digits.length <= DNI_MAX_DIGITS) return dniShape(digits);
  return cuitShape(digits);
}

// The literal that follows `count` digits: a CUIT's hyphen after its prefix and after its body, a
// DNI's dot where a seven- or eight-digit number groups its thousands. A `cuit-or-dni` takes the
// literal of the shape its next digit gives it, so its only hyphen comes after the tenth digit.
// A DNI groups from the right, so a typed `12.` + `3` reads `123` until the fourth digit regroups it.
function separatorAfter(count: number, kind: TaxIdKind): string | null {
  const next = kind === 'cuit-or-dni' ? (count >= DNI_MAX_DIGITS ? 'cuit' : 'dni') : kind;
  if (next === 'cuit') {
    return count === CUIT_PREFIX_DIGITS || count === CUIT_BODY_END ? CUIT_SEPARATOR : null;
  }
  const grouped = [DNI_MIN_DIGITS, DNI_MAX_DIGITS].some(
    (length) => length > count && (length - count) % DNI_GROUP === 0,
  );
  return grouped ? DNI_SEPARATOR : null;
}

const isDigit = (char: string) => char >= '0' && char <= '9';
const isSeparator = (char: string) => char === CUIT_SEPARATOR || char === DNI_SEPARATOR;

// The position just after the `count`-th digit; 0 when `count` is 0.
function caretAfterDigits(display: string, count: number): number {
  if (count <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < display.length; i += 1) {
    if (isDigit(display[i] ?? '')) seen += 1;
    if (seen === count) return i + 1;
  }
  return display.length;
}

export interface TaxIdEdit {
  digits: string;
  display: string;
  caret: number;
}

// Reads the field after any edit back into digits, display and a caret mapped by counting digits;
// a full field refuses more digits rather than pushing its last ones out.
export function editTaxId(
  raw: string,
  caret: number,
  previousDisplay: string,
  kind: TaxIdKind,
): TaxIdEdit {
  const max = TAX_ID_MAX_DIGITS[kind];
  const previous = taxIdDigits(previousDisplay);
  const typed = taxIdDigits(raw);
  if (typed.length > max && previous.length >= max) {
    const inserted = raw.length - previousDisplay.length;
    return { digits: previous, display: previousDisplay, caret: Math.max(0, caret - inserted) };
  }
  const digits = typed.slice(0, max);
  const separator = separatorAfter(digits.length, kind);
  const trailing = raw.replace(/^.*\d/, '');
  const display =
    formatTaxId(digits, kind) + (separator && trailing.includes(separator) ? separator : '');

  const beforeCaret = raw.slice(0, caret);
  let at = caretAfterDigits(display, Math.min(taxIdDigits(beforeCaret).length, digits.length));
  [...beforeCaret.replace(/^.*\d/, '')].forEach((char) => {
    if (isSeparator(char) && display[at] === char) at += 1;
  });
  return { digits, display, caret: at };
}

function lastDigitBefore(display: string, end: number): number {
  for (let i = end - 1; i >= 0; i -= 1) if (isDigit(display[i] ?? '')) return i;
  return -1;
}

// Backspace or Delete against a separator inside the number takes the digit beyond it, since the
// format would put a lone separator straight back; null leaves the key to the browser.
export function deleteAcrossSeparator(
  display: string,
  caret: number,
  key: string,
  kind: TaxIdKind,
): TaxIdEdit | null {
  let removed = -1;
  if (key === 'Backspace' && caret < display.length && isSeparator(display[caret - 1] ?? '')) {
    removed = lastDigitBefore(display, caret - 1);
  } else if (key === 'Delete' && caret + 1 < display.length && isSeparator(display[caret] ?? '')) {
    removed = display.slice(caret + 1).search(/\d/);
    if (removed >= 0) removed += caret + 1;
  }
  if (removed < 0) return null;
  const raw = display.slice(0, removed) + display.slice(removed + 1);
  return editTaxId(raw, key === 'Backspace' ? removed : caret, display, kind);
}

// What a paste may add to the field: its digits up to the room left, with the separators among them.
export function pastedTaxId(text: string, room: number): string {
  let kept = '';
  let digits = 0;
  for (const char of text) {
    if (isDigit(char)) {
      if (digits === room) break;
      digits += 1;
    } else if (!isSeparator(char)) {
      continue;
    }
    kept += char;
  }
  return kept;
}

/*
 * Drops every character `allowed` (a one-character, non-global pattern) refuses and keeps the caret
 * after the characters it followed — for a number whose shape varies too much to format, like a
 * provincial gross income registration.
 */
export function keepAllowed(
  raw: string,
  caret: number,
  allowed: RegExp,
): { value: string; caret: number } {
  const keeps = (char: string) => allowed.test(char);
  return {
    value: [...raw].filter(keeps).join(''),
    caret: [...raw.slice(0, caret)].filter(keeps).length,
  };
}
