/*
 * The colour the client webapp renders a quote with. A mirror of what the API accepts on
 * `brand_color` (`hexcolor`): three, four, six or eight digits behind a hash, so four and eight
 * carry an alpha channel. Kept identical rather than narrower — a form that refuses what the API
 * stores is a form that cannot show the account its own value back.
 */
export const HEX_COLOR = /^#(?:[0-9a-fA-F]{3,4}){1,2}$/;
