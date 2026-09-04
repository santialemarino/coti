/*
 * The colour the client webapp renders a quote with. A mirror of what the API accepts on
 * `brand_color` (`hexcolor`): three, four, six or eight digits behind a hash, so four and eight
 * carry an alpha channel. Kept identical rather than narrower — a form that refuses what the API
 * stores is a form that cannot show the account its own value back.
 */
export const HEX_COLOR = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

/* The onboarding field renders the hash as a fixed prefix, so its editable value carries only
 * the hexadecimal digits accepted by the API. */
export const HEX_COLOR_DIGITS = /^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

export const DEFAULT_BRAND_COLOR = '#2F6CB3';
