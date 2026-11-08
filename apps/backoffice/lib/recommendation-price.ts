type RecommendationKind = 'ECONOMY' | 'PREMIUM';

function decimalParts(value: string): [string, string] | null {
  if (!/^\d+(?:\.\d{1,2})?$/.test(value)) return null;
  const [whole = '', fraction = ''] = value.split('.');
  return [whole.replace(/^0+(?=\d)/, ''), fraction.padEnd(2, '0')];
}

/** Checks the displayed prices without converting decimal amounts to floating point. */
export function recommendationPriceValid(
  basePrice: string | null,
  alternativePrice: string | null,
  type: RecommendationKind,
): boolean {
  if (basePrice === null || alternativePrice === null) return false;
  const base = decimalParts(basePrice);
  const alternative = decimalParts(alternativePrice);
  if (!base || !alternative) return false;
  const comparison =
    alternative[0].length - base[0].length ||
    alternative[0].localeCompare(base[0]) ||
    alternative[1].localeCompare(base[1]);
  return type === 'ECONOMY' ? comparison < 0 : comparison > 0;
}
