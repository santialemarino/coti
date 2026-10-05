import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { FitLabel } from '@/app/(protected)/_components/fit-label';

const layout = vi.hoisted(() => ({ room: 0, text: 0, notify: () => {}, notifyText: () => {} }));

// jsdom lays nothing out, so the room and the text's natural width are set by hand.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (
    this: HTMLElement,
  ) {
    const width = this.getAttribute('aria-hidden') === 'true' ? layout.text : layout.room;
    return { width } as DOMRect;
  });
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(private callback: () => void) {}
      // The box changing is the room changing; the probe changing is the text's own width (a font).
      observe(target: Element) {
        if (target.getAttribute('aria-hidden') === 'true') layout.notifyText = this.callback;
        else layout.notify = this.callback;
      }
      disconnect() {}
    },
  );
});

afterEach(() => {
  layout.notify = () => {};
  layout.notifyText = () => {};
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function visible() {
  return screen.getByText((_, element) => element?.classList.contains('relative') === true)
    .lastChild?.textContent;
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

  it('keeps the full text when it fills the room exactly', () => {
    Object.assign(layout, { room: 140, text: 140 });
    render(<FitLabel full="Todas las sucursales" short="Todas" />);

    expect(visible()).toBe('Todas las sucursales');
  });

  // A font arriving late widens the text without the room changing.
  it('gives way to the short text when the full one grows past its room', () => {
    Object.assign(layout, { room: 160, text: 140 });
    render(<FitLabel full="Todas las sucursales" short="Todas" />);

    layout.text = 180;
    act(() => layout.notifyText());

    expect(visible()).toBe('Todas');
  });
});
