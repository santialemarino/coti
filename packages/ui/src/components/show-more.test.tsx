import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ShowMore } from './show-more';

const labels = { status: 'Mostrando 2 de 7', more: 'Ver 2 más', all: 'Ver todas (7)' };

describe('ShowMore', () => {
  it('offers the next step and everything while more than one step remains', () => {
    const onShowMore = vi.fn();
    const onShowAll = vi.fn();
    const view = render(
      <ShowMore
        visible={2}
        total={7}
        step={2}
        onShowMore={onShowMore}
        onShowAll={onShowAll}
        labels={labels}
      />,
    );
    expect(view.getByRole('status').textContent).toBe('Mostrando 2 de 7');

    fireEvent.click(view.getByRole('button', { name: 'Ver 2 más' }));
    fireEvent.click(view.getByRole('button', { name: 'Ver todas (7)' }));
    expect(onShowMore).toHaveBeenCalledOnce();
    expect(onShowAll).toHaveBeenCalledOnce();
  });

  // One step reaching the end already shows everything, so a second button would do the same.
  it('offers only the next step once it reaches the end', () => {
    const view = render(
      <ShowMore
        visible={6}
        total={7}
        step={2}
        onShowMore={() => {}}
        onShowAll={() => {}}
        labels={labels}
      />,
    );
    expect(view.getAllByRole('button')).toHaveLength(1);
  });

  it('offers only the next step when exactly one step remains', () => {
    const view = render(
      <ShowMore
        visible={5}
        total={7}
        step={2}
        onShowMore={() => {}}
        onShowAll={() => {}}
        labels={labels}
      />,
    );
    expect(view.getAllByRole('button')).toHaveLength(1);
  });

  it('renders nothing once the whole list shows', () => {
    const view = render(
      <ShowMore
        visible={7}
        total={7}
        step={2}
        onShowMore={() => {}}
        onShowAll={() => {}}
        labels={labels}
      />,
    );
    expect(view.container.innerHTML).toBe('');
  });
});
