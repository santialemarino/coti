/* The API accepts three, four, six or eight digits behind a hash. Brand fields render that hash
 * as a fixed prefix, so their editable value carries only the hexadecimal digits. */
export const HEX_COLOR_DIGITS = /^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

export const DEFAULT_BRAND_COLOR = '#2F6CB3';
