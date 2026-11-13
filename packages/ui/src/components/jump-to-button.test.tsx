import * as React from 'react';
import { act, fireEvent, render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { JumpToButton } from './jump-to-button';

// The flight runs to its end at once, so a test reads where it lands and what takes focus.
const flight = vi.hoisted(() => ({ hold: false, stop: () => {} }));

vi.mock('motion/react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('motion/react')>()),
  animate: (
    _from: number,
    to: number,
    options: { onUpdate: (value: number) => void; onComplete: () => void },
  ) => {
    if (!flight.hold) {
      options.onUpdate(to);
      options.onComplete();
    }
    return { stop: () => flight.stop() };
  },
}));

let report: (entry: { isIntersecting: boolean; top: number }) => void = () => {};

class ObserverStub {
  constructor(callback: IntersectionObserverCallback) {
    report = ({ isIntersecting, top }) =>
      callback(
        [{ isIntersecting, boundingClientRect: { top } } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
  }
  observe() {}
  disconnect() {}
}

function Page({ enabled = true }: { enabled?: boolean }) {
  const target = React.useRef<HTMLDivElement>(null);
  return (
    <>
      <div ref={target} data-testid="actions">
        <button disabled={!enabled}>Elegir otro archivo</button>
        <button disabled={!enabled}>Confirmar</button>
      </div>
      <JumpToButton target={target} label="Ir a confirmar" />
    </>
  );
}

function wrapper(view: ReturnType<typeof render>) {
  return view.container.querySelector<HTMLElement>('[data-slot="jump-to-button"]');
}

beforeEach(() => {
  vi.stubGlobal('IntersectionObserver', ObserverStub);
  window.innerHeight = 800;
  Object.defineProperty(document.documentElement, 'scrollHeight', {
    configurable: true,
    value: 5000,
  });
  window.scrollTo = vi.fn() as unknown as typeof window.scrollTo;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('JumpToButton', () => {
  it('stays hidden and out of the tab order while the target is on screen', () => {
    const view = render(<Page />);
    act(() => report({ isIntersecting: true, top: 300 }));
    expect(wrapper(view)?.dataset.state).toBe('hidden');
    expect(wrapper(view)?.hasAttribute('inert')).toBe(true);
  });

  it('shows while the target is below the screen, and points down', () => {
    const view = render(<Page />);
    act(() => report({ isIntersecting: false, top: 2400 }));
    expect(wrapper(view)?.dataset.state).toBe('visible');
    expect(wrapper(view)?.hasAttribute('inert')).toBe(false);
    expect(view.container.querySelector('svg')?.getAttribute('class')).toContain('rotate-0');
  });

  it('points up when the target is above the screen', () => {
    const view = render(<Page />);
    act(() => report({ isIntersecting: false, top: -900 }));
    expect(view.container.querySelector('svg')?.getAttribute('class')).toContain('rotate-180');
  });

  // Scrolling the target's bottom edge into view past a page end would ease into a wall.
  it('lands on the target, held to the end of the page, and focuses its action', () => {
    const view = render(<Page />);
    vi.spyOn(view.getByTestId('actions'), 'getBoundingClientRect').mockReturnValue({
      top: 4900,
      bottom: 4950,
    } as DOMRect);
    act(() => report({ isIntersecting: false, top: 4900 }));

    fireEvent.click(view.getByRole('button', { name: 'Ir a confirmar' }));
    expect(window.scrollTo).toHaveBeenLastCalledWith(0, 4200);
    expect(document.activeElement).toBe(view.getByRole('button', { name: 'Confirmar' }));
  });

  it('scrolls up to the target with room under the sticky header', () => {
    const view = render(<Page />);
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 3000 });
    vi.spyOn(view.getByTestId('actions'), 'getBoundingClientRect').mockReturnValue({
      top: -2000,
      bottom: -1950,
    } as DOMRect);
    act(() => report({ isIntersecting: false, top: -2000 }));

    fireEvent.click(view.getByRole('button', { name: 'Ir a confirmar' }));
    expect(window.scrollTo).toHaveBeenLastCalledWith(0, 912);
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 });
  });

  // Focus left on the arrow would drop to the page once it hides, so the row itself takes it.
  it('focuses the target itself when none of its actions is enabled', () => {
    const view = render(<Page enabled={false} />);
    act(() => report({ isIntersecting: false, top: 2400 }));
    fireEvent.click(view.getByRole('button', { name: 'Ir a confirmar' }));
    expect(document.activeElement).toBe(view.getByTestId('actions'));
  });

  // A scroll by hand mid-flight wins; the flight must not drag the page back.
  it.each(['wheel', 'keydown', 'pointerdown'])('stops the flight on %s', (type) => {
    flight.hold = true;
    flight.stop = vi.fn();
    const view = render(<Page />);
    act(() => report({ isIntersecting: false, top: 2400 }));
    fireEvent.click(view.getByRole('button', { name: 'Ir a confirmar' }));
    window.dispatchEvent(new Event(type));
    expect(flight.stop).toHaveBeenCalled();
    flight.hold = false;
  });
});
