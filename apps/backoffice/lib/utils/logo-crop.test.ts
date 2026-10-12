import { describe, expect, it } from 'vitest';

import { logoCropGeometry } from '@/lib/utils/logo-crop';

describe('logoCropGeometry', () => {
  it('preserves an entire vertical logo in a horizontal frame without distortion', () => {
    expect(logoCropGeometry(100, 400, 600, 200, 1, 0, 0)).toEqual({
      width: 50,
      height: 200,
      x: 275,
      y: 0,
    });
  });
  it('constrains zoomed panning to the image edges', () => {
    expect(logoCropGeometry(400, 100, 400, 100, 2, 10, -10)).toEqual({
      width: 800,
      height: 200,
      x: 0,
      y: -100,
    });
  });
  it('keeps unused transparent space centered while zooming a square logo', () => {
    expect(logoCropGeometry(100, 100, 600, 200, 2, 10, 0)).toEqual({
      width: 400,
      height: 400,
      x: 100,
      y: -100,
    });
  });
});
