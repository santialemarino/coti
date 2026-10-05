import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Stepper } from './stepper';

const STEPS = [
  { id: 'a', label: 'Recibido', meta: '04/10/2026' },
  { id: 'b', label: 'Generado', meta: '04/10/2026' },
  { id: 'c', label: 'Cotizado' },
];

function steps(orientation?: 'horizontal' | 'adaptive') {
  render(<Stepper steps={STEPS} currentIndex={1} orientation={orientation} />);
  return screen.getAllByRole('listitem');
}

describe('Stepper', () => {
  it('marks the step in progress', () => {
    const items = steps();

    expect(items[1]?.getAttribute('aria-current')).toBe('step');
    expect(items[0]?.getAttribute('aria-current')).toBeNull();
  });

  // jsdom evaluates no breakpoint, so this pins the rules; the layout itself is checked in a browser.
  it('runs down the screen below sm only when adaptive', () => {
    const adaptive = steps('adaptive');
    expect(adaptive[0]?.closest('ol')?.classList.contains('max-sm:flex-col')).toBe(true);
    expect(adaptive[0]?.classList.contains('max-sm:grid')).toBe(true);
  });

  it('stays a row at every width by default', () => {
    const row = steps();
    expect(row[0]?.closest('ol')?.classList.contains('max-sm:flex-col')).toBe(false);
    expect(row[0]?.classList.contains('max-sm:grid')).toBe(false);
  });
});
