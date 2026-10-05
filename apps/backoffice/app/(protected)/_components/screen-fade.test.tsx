import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ScreenFade } from '@/app/(protected)/_components/screen-fade';

const navigation = vi.hoisted(() => ({ pathname: '/rfqs/a' }));

vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));

function frame() {
  return screen.getByText('pantalla').parentElement as HTMLElement;
}

describe('ScreenFade', () => {
  // One order replacing another stays inside the same section; it must fade all the same.
  it('starts the fade again whenever the path changes, within a section too', () => {
    const view = render(
      <ScreenFade>
        <p>pantalla</p>
      </ScreenFade>,
    );
    const first = frame();

    navigation.pathname = '/rfqs/b';
    view.rerender(
      <ScreenFade>
        <p>pantalla</p>
      </ScreenFade>,
    );

    expect(frame()).not.toBe(first);
    expect(frame().className).toContain('fade-in-0');
  });
});
