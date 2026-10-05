import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ScreenFade } from '@/app/(protected)/_components/screen-fade';

const navigation = vi.hoisted(() => ({ pathname: '/rfqs/a' }));

vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));
// Next's own React carries <ViewTransition>; the package the tests resolve does not yet.
vi.mock('react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react')>()),
  ViewTransition: vi.fn(({ children }: { children: React.ReactNode }) => children),
}));

const { ViewTransition } = await import('react');

function frame() {
  return screen.getByText('pantalla').parentElement as HTMLElement;
}

function renderAt(pathname: string) {
  navigation.pathname = pathname;
  return (
    <ScreenFade>
      <p>pantalla</p>
    </ScreenFade>
  );
}

describe('ScreenFade', () => {
  // The `screen` rules in @repo/ui animate only this name, and only on a swap — never on an update.
  it('names the boundary the fade rules target and animates swaps only', () => {
    render(renderAt('/rfqs/a'));

    expect(vi.mocked(ViewTransition).mock.calls.at(-1)?.[0]).toMatchObject({
      name: 'screen',
      default: 'none',
      share: 'auto',
    });
  });

  // One order replacing another stays inside the same section; it must swap all the same.
  it('swaps the screen whenever the path changes, within a section too', () => {
    const view = render(renderAt('/rfqs/a'));
    const first = frame();

    view.rerender(renderAt('/rfqs/b'));

    expect(frame()).not.toBe(first);
  });
});
