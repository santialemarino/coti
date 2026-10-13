import { CUIT_DIGITS } from '@/lib/utils/cuit';

export type TaxIdKind = 'cuit' | 'dni' | 'cuit-or-dni';

const DNI_MIN_DIGITS = 7;
const DNI_MAX_DIGITS = 8;
const CUIT_PREFIX_DIGITS = 2;
const CUIT_BODY_END = 10;
const DNI_GROUP = 3;

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
  return check ? `${prefix}-${body}-${check}` : `${prefix}-${body}`;
}

// Grouped in thousands from the right, so a DNI reads the same while typed as once complete.
function dniShape(digits: string): string {
  return digits.replace(new RegExp(`\\B(?=(\\d{${DNI_GROUP}})+$)`, 'g'), '.');
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

// The position just after the `count`-th character `counts` accepts; 0 when `count` is 0.
function caretAfter(display: string, count: number, counts: (char: string) => boolean): number {
  if (count <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < display.length; i += 1) {
    if (counts(display[i] ?? '')) seen += 1;
    if (seen === count) return i + 1;
  }
  return display.length;
}

const isDigit = (char: string) => char >= '0' && char <= '9';

function countDigits(text: string): number {
  return taxIdDigits(text).length;
}

export interface TaxIdEdit {
  digits: string;
  display: string;
  caret: number;
}

/*
 * Reads what the field holds after an edit (a keystroke, a paste, autofill) back into digits and a
 * caret. The caret is mapped by counting digits, because separators are exactly what moved. A field
 * already full refuses more digits instead of pushing its last ones out; text that overflows an
 * emptier one keeps its first digits.
 */
export function editTaxId(
  raw: string,
  caret: number,
  previous: string,
  kind: TaxIdKind,
): TaxIdEdit {
  const max = TAX_ID_MAX_DIGITS[kind];
  const typed = taxIdDigits(raw);
  if (typed.length > max && previous.length >= max) {
    const display = formatTaxId(previous, kind);
    const inserted = raw.length - display.length;
    return { digits: previous, display, caret: Math.max(0, caret - inserted) };
  }
  const digits = typed.slice(0, max);
  const display = formatTaxId(digits, kind);
  const digitsBefore = Math.min(countDigits(raw.slice(0, caret)), digits.length);
  return { digits, display, caret: caretAfter(display, digitsBefore, isDigit) };
}

/*
 * Backspace or Delete against a separator takes the digit beyond it: removing the separator alone
 * would be put straight back by the format, trapping the caret. Null when the key is not that case.
 */
export function deleteAcrossSeparator(
  display: string,
  caret: number,
  key: string,
  kind: TaxIdKind,
): TaxIdEdit | null {
  const digits = taxIdDigits(display);
  const before = countDigits(display.slice(0, caret));
  let removed: number;
  if (key === 'Backspace' && caret > 0 && !isDigit(display[caret - 1] ?? '')) {
    if (before === 0) return null;
    removed = before - 1;
  } else if (key === 'Delete' && caret < display.length && !isDigit(display[caret] ?? '')) {
    if (before === digits.length) return null;
    removed = before;
  } else {
    return null;
  }
  const next = digits.slice(0, removed) + digits.slice(removed + 1);
  const shown = formatTaxId(next, kind);
  return { digits: next, display: shown, caret: caretAfter(shown, removed, isDigit) };
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
