import { render, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useScrollLane } from './use-scroll-lane';

function List() {
  const lane = useScrollLane<HTMLDivElement>();
  return <div data-testid="list" ref={lane} />;
}

// jsdom lays nothing out, so the list's two heights are set by hand and a child is added to re-check.
function sized(element: HTMLElement, scrollHeight: number, clientHeight: number) {
  Object.defineProperty(element, 'scrollHeight', { configurable: true, value: scrollHeight });
  Object.defineProperty(element, 'clientHeight', { configurable: true, value: clientHeight });
  element.append(document.createElement('div'));
}

describe('useScrollLane', () => {
  it('marks the list while its content overflows, and unmarks it once it fits', async () => {
    const view = render(<List />);
    const list = view.getByTestId('list');
    expect(list.hasAttribute('data-scroll-lane')).toBe(false);

    sized(list, 600, 256);
    await waitFor(() => expect(list.hasAttribute('data-scroll-lane')).toBe(true));

    sized(list, 200, 256);
    await waitFor(() => expect(list.hasAttribute('data-scroll-lane')).toBe(false));
  });
});
