import { describe, expect, it } from 'vitest';

import { recommendationPriceValid } from '@/lib/recommendation-price';

describe('recommendationPriceValid', () => {
  it('requires economy to be cheaper and premium to be dearer', () => {
    expect(recommendationPriceValid('9256.00', '8900.00', 'ECONOMY')).toBe(true);
    expect(recommendationPriceValid('9256.00', '12000.00', 'PREMIUM')).toBe(true);
    expect(recommendationPriceValid('9256.00', '9256.00', 'ECONOMY')).toBe(false);
    expect(recommendationPriceValid('9256.00', '8900.00', 'PREMIUM')).toBe(false);
  });

  it('compares decimal strings exactly and refuses unavailable prices', () => {
    expect(recommendationPriceValid('999999999999.99', '999999999999.98', 'ECONOMY')).toBe(true);
    expect(recommendationPriceValid('0009.5', '9.50', 'PREMIUM')).toBe(false);
    expect(recommendationPriceValid(null, '10.00', 'ECONOMY')).toBe(false);
    expect(recommendationPriceValid('10.00', 'invalid', 'PREMIUM')).toBe(false);
  });
});
