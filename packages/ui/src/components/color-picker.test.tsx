import { describe, expect, it } from 'vitest';

import { hexToHsv, hsvToHex } from './color-picker';

describe('colour conversion', () => {
  /*
   * The pad and the rail write HSV and the field reads hex, so anything the round trip loses shows
   * up as a colour that drifts while nobody is touching it.
   */
  it('round-trips every corner of the cube', () => {
    for (const hex of ['FFFFFF', '000000', 'FF0000', '00FF00', '0000FF', '2F6CB3', 'C2410C']) {
      expect(hsvToHex(hexToHsv(hex)), hex).toBe(hex);
    }
  });

  it('reads the hue of each primary from its hex', () => {
    expect(hexToHsv('FF0000').h).toBe(0);
    expect(hexToHsv('00FF00').h).toBe(120);
    expect(hexToHsv('0000FF').h).toBe(240);
  });

  // Grey has no hue to recover, which is why the component keeps the last one it saw.
  it('reports no saturation for grey, whatever its brightness', () => {
    expect(hexToHsv('808080').s).toBe(0);
    expect(hexToHsv('000000').s).toBe(0);
  });
});
