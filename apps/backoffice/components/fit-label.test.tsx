import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { FitLabel } from '@/components/fit-label';

const layout = vi.hoisted(() => ({ room: 0, text: 0, notify: () => {} }));

// jsdom lays nothing out, so the room and the text's natural width are set by hand.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => layout.room);
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockImplementation(() => layout.text);
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(private callback: () => void) {}
      // Only a change to the box itself is the room changing.
      observe(target: Element) {
        if (target.className === 'block relative') layout.notify = this.callback;
      }
      disconnect() {}
    },
  );
});

afterEach(() => {
  layout.notify = () => {};
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function visible() {
  return screen.getByText((_, element) => element?.className === 'block relative').lastChild
    ?.textContent;
}

describe('FitLabel', () => {
  it('shows the full text while it fits', () => {
    Object.assign(layout, { room: 160, text: 140 });
    render(<FitLabel full="Todas las sucursales" short="Todas" />);

    expect(visible()).toBe('Todas las sucursales');
  });

  it('shows the short text once the full one would be cut', () => {
    Object.assign(layout, { room: 100, text: 140 });
    render(<FitLabel full="Todas las sucursales" short="Todas" />);

    expect(visible()).toBe('Todas');
  });

  // The full text is measured on its copy, so the short form can give way again when room returns.
  it('goes back to the full text when the room grows', () => {
    Object.assign(layout, { room: 100, text: 140 });
    render(<FitLabel full="Todas las sucursales" short="Todas" />);

    layout.room = 200;
    act(() => layout.notify());

    expect(visible()).toBe('Todas las sucursales');
  });
});
