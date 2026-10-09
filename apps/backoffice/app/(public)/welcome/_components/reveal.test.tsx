import { render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { Reveal } from '@/app/(public)/welcome/_components/reveal';

let intersect: (isIntersecting: boolean) => void = () => {};
let reducedMotion = false;

class ObserverStub {
  constructor(callback: (entries: { isIntersecting: boolean }[]) => void) {
    intersect = (isIntersecting) => callback([{ isIntersecting }]);
  }
  observe() {}
  disconnect() {}
}

function renderAt(top: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ top } as DOMRect);
  const view = render(<Reveal>sección</Reveal>);
  return view.getByText('sección');
}

beforeEach(() => {
  reducedMotion = false;
  vi.stubGlobal('IntersectionObserver', ObserverStub);
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: reducedMotion })),
  );
  window.innerHeight = 800;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('Reveal', () => {
  it('holds a section below the fold and shows it once it enters the screen', () => {
    const section = renderAt(1200);
    expect(section.dataset.reveal).toBe('waiting');

    intersect(true);
    expect(section.dataset.reveal).toBe('shown');
  });

  // Once shown it stays shown: scrolling back past it must not hide it again.
  it('never hides a section it has already shown', () => {
    const section = renderAt(1200);
    intersect(true);
    intersect(false);

    expect(section.dataset.reveal).toBe('shown');
  });

  it('leaves a section the page opened on as it is', () => {
    expect(renderAt(200).dataset.reveal).toBeUndefined();
  });

  it('leaves every section as it is under reduced motion', () => {
    reducedMotion = true;
    expect(renderAt(1200).dataset.reveal).toBeUndefined();
  });
});
