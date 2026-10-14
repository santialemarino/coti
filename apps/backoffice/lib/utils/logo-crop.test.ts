import { describe, expect, it } from 'vitest';

import { clampLogoOffset, logoCropGeometry } from '@/lib/utils/logo-crop';

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

describe('clampLogoOffset', () => {
  it('holds the offset at the edge the zoomed image can reach', () => {
    // 800×200 drawn in a 400×100 frame travels 200px either way: half the frame.
    expect(clampLogoOffset(400, 100, 400, 100, 2, { x: 3, y: -3 })).toEqual({ x: 0.5, y: -0.5 });
  });

  it('pins an image that fits the frame to its centre', () => {
    expect(clampLogoOffset(100, 100, 600, 200, 1, { x: 0.4, y: 0.2 })).toEqual({ x: 0, y: 0 });
  });

  it('leaves an offset inside the range alone', () => {
    expect(clampLogoOffset(400, 100, 400, 100, 2, { x: 0.25, y: 0 })).toEqual({ x: 0.25, y: 0 });
  });
});
