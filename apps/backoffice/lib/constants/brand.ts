/* The API accepts three, four, six or eight digits behind a hash. Brand fields render that hash
 * as a fixed prefix, so their editable value carries only the hexadecimal digits. */
export const HEX_COLOR_DIGITS = /^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

export const DEFAULT_BRAND_COLOR = '#2F6CB3';

/*
 * One-click choices in the colour picker. The first is Coti's own primary; the rest are the colours
 * a corralón's signage actually uses, so the common case is a click rather than a hex hunt.
 */
export const BRAND_COLOR_PRESETS: readonly string[] = [
  '2F6CB3',
  '1D3558',
  '0F766E',
  '15803D',
  'C2410C',
  'B91C1C',
  'A16207',
  '6D28D9',
  '334155',
  '111924',
];
