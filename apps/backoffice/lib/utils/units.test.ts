import { describe, expect, it } from 'vitest';

import { shortUnit } from './units';

describe('shortUnit', () => {
  it.each([
    ['unidades', 'u.'],
    ['Unidad', 'u.'],
    ['bolsas', 'bol.'],
    ['barras', 'bar.'],
    ['m3', 'm³'],
    ['metros cúbicos', 'm³'],
    ['M2', 'm²'],
    ['metros  lineales', 'ml'],
    ['kilos', 'kg'],
    ['litros', 'l'],
    ['placas', 'pl.'],
  ])('abbreviates %s as %s', (unit, short) => {
    expect(shortUnit(unit)).toBe(short);
  });

  it('keeps a unit it does not know as written, never cut', () => {
    expect(shortUnit(' rollos de 10 m2 ')).toBe('rollos de 10 m2');
    expect(shortUnit('pallet')).toBe('pallet');
  });
});
