/*
 * The trade abbreviation a quote line shows for its unit. The unit is free text — the customer's
 * word or the catalogue's — so this only names the common ones; anything else is shown as written.
 * It is display only: the line keeps the original, which the abbreviation carries as its title.
 */
const ABBREVIATIONS: Record<string, string> = {
  u: 'u.',
  un: 'u.',
  unidad: 'u.',
  unidades: 'u.',
  bolsa: 'bol.',
  bolsas: 'bol.',
  barra: 'bar.',
  barras: 'bar.',
  placa: 'pl.',
  placas: 'pl.',
  ml: 'ml',
  'metro lineal': 'ml',
  'metros lineales': 'ml',
  m2: 'm²',
  'metro cuadrado': 'm²',
  'metros cuadrados': 'm²',
  m3: 'm³',
  'metro cubico': 'm³',
  'metros cubicos': 'm³',
  kg: 'kg',
  kilo: 'kg',
  kilos: 'kg',
  kilogramo: 'kg',
  kilogramos: 'kg',
  l: 'l',
  lt: 'l',
  litro: 'l',
  litros: 'l',
};

// The unit as the line shows it: its trade abbreviation when there is one, otherwise as written.
export function shortUnit(unit: string): string {
  const key = unit
    .trim()
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/\.$/, '')
    .replace(/\s+/g, ' ');
  return ABBREVIATIONS[key] ?? unit.trim();
}
