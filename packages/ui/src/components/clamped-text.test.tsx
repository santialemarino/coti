import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { ClampedText } from './clamped-text';

const LONG = 'Ladrillo cerámico hueco 12×18×33 de segunda selección';

// jsdom lays nothing out, so the element reports whatever height the test gives it.
function layout(scrollHeight: number, clientHeight: number) {
  Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
    configurable: true,
    get: () => scrollHeight,
  });
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
    configurable: true,
    get: () => clientHeight,
  });
}

describe('ClampedText', () => {
  afterEach(() => layout(0, 0));

  it('stays plain text when nothing is cut', () => {
    layout(40, 40);
    render(<ClampedText>{LONG}</ClampedText>);

    const text = screen.getByText(LONG);
    expect(text.getAttribute('tabindex')).toBeNull();
    expect(text.className).toContain('line-clamp-2');
  });

  it('takes focus and shows the full text when it is cut', async () => {
    layout(60, 40);
    render(<ClampedText>{LONG}</ClampedText>);

    const text = screen.getByText(LONG);
    expect(text.getAttribute('tabindex')).toBe('0');
    await act(async () => {
      fireEvent.focus(text);
    });
    expect(screen.getByRole('tooltip').textContent).toContain(LONG);
  });
});
