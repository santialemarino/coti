import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useProgressiveList } from './use-progressive-list';

function List({ total, step }: { total: number; step: number }) {
  const list = useProgressiveList<HTMLLIElement>(total, step);
  return (
    <>
      <ul>
        {Array.from({ length: list.visible }, (_, index) => (
          <li
            key={index}
            ref={index === list.revealedFrom ? list.revealedRef : undefined}
            tabIndex={index === list.revealedFrom ? -1 : undefined}
          >
            {`item ${index + 1}`}
          </li>
        ))}
      </ul>
      {list.visible < total ? (
        <>
          <button onClick={list.showMore}>more</button>
          <button onClick={list.showAll}>all</button>
        </>
      ) : null}
    </>
  );
}

describe('useProgressiveList', () => {
  it('shows one step, then the next, then everything at once', () => {
    const view = render(<List total={7} step={2} />);
    expect(view.getAllByRole('listitem')).toHaveLength(2);

    fireEvent.click(view.getByText('more'));
    expect(view.getAllByRole('listitem')).toHaveLength(4);

    fireEvent.click(view.getByText('all'));
    expect(view.getAllByRole('listitem')).toHaveLength(7);
  });

  it('never shows more than the list holds', () => {
    const view = render(<List total={3} step={2} />);
    fireEvent.click(view.getByText('more'));
    expect(view.getAllByRole('listitem')).toHaveLength(3);
  });

  // The pressed control disappears once everything shows, so focus must land on the list instead.
  it('moves focus onto the first item each reveal adds', () => {
    const view = render(<List total={7} step={2} />);
    fireEvent.click(view.getByText('more'));
    expect(document.activeElement).toBe(view.getByText('item 3'));

    fireEvent.click(view.getByText('all'));
    expect(document.activeElement).toBe(view.getByText('item 5'));
  });
});
