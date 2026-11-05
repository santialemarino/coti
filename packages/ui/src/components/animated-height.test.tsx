import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { AnimatedHeight } from './animated-height';

describe('AnimatedHeight', () => {
  it('renders its content inside a box that clips without cutting the elevation', () => {
    const view = render(
      <AnimatedHeight trigger="one">
        <p>Paso uno</p>
      </AnimatedHeight>,
    );
    const box = view.getByText('Paso uno').parentElement?.parentElement;
    expect(box?.className).toContain('overflow-clip');
    expect(box?.className).toContain('[overflow-clip-margin:0.5rem]');
    expect(box?.style.height).toBe('');
  });
});
