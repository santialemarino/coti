import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { SurfaceFade } from '@/components/surface-fade';

const navigation = vi.hoisted(() => ({ pathname: '/' }));
vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));

function view(pathname: string) {
  navigation.pathname = pathname;
  return render(<SurfaceFade>contenido</SurfaceFade>);
}

function wrapper(container: HTMLElement) {
  return container.firstElementChild as HTMLElement;
}

beforeEach(() => {
  navigation.pathname = '/';
});

describe('SurfaceFade', () => {
  // A page load is not a crossing; fading the first paint would only delay it.
  it('does not fade the page it first renders', () => {
    const { container } = view('/');

    expect(wrapper(container).className).not.toContain('animate-surface-in');
  });

  it('fades in when a navigation crosses into another surface', () => {
    const { container, rerender } = view('/');
    navigation.pathname = '/login';
    rerender(<SurfaceFade>contenido</SurfaceFade>);

    expect(wrapper(container).className).toContain('animate-surface-in');
  });

  // Moving inside the app is the screen swap's job; the page fade must stay out of it.
  it('stays still while moving within one surface', () => {
    const { container, rerender } = view('/inbox');
    navigation.pathname = '/clients';
    rerender(<SurfaceFade>contenido</SurfaceFade>);

    expect(wrapper(container).className).not.toContain('animate-surface-in');
  });

  // A second crossing must fade again, which takes a fresh element for the animation to start on.
  it('starts the fade over on every crossing', () => {
    const { container, rerender } = view('/');
    navigation.pathname = '/login';
    rerender(<SurfaceFade>contenido</SurfaceFade>);
    const first = wrapper(container);
    navigation.pathname = '/inbox';
    rerender(<SurfaceFade>contenido</SurfaceFade>);

    expect(wrapper(container)).not.toBe(first);
    expect(wrapper(container).className).toContain('animate-surface-in');
  });
});
