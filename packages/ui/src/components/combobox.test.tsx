import * as React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Combobox } from './combobox';

const OPTIONS = [
  { value: 'AR', label: 'Argentina (+54)' },
  { value: 'UY', label: 'Uruguay (+598)' },
];

describe('Combobox', () => {
  // A form focuses the first invalid field through its ref, so the ref has to reach the trigger.
  it('hands a passed ref the trigger', () => {
    const ref = React.createRef<HTMLButtonElement>();
    render(
      <Combobox
        options={OPTIONS}
        value={null}
        onValueChange={() => {}}
        placeholder="País"
        ref={ref}
      />,
    );

    expect(ref.current).toBe(screen.getByRole('combobox'));
  });

  it('fills the trigger like a field', () => {
    render(<Combobox options={OPTIONS} value={null} onValueChange={() => {}} placeholder="País" />);

    expect(screen.getByRole('combobox').classList.contains('bg-input')).toBe(true);
  });

  it('names the trigger after the chosen option', () => {
    render(<Combobox options={OPTIONS} value="UY" onValueChange={() => {}} placeholder="País" />);

    expect(screen.getByRole('combobox').textContent).toBe('Uruguay (+598)');
  });

  it('renders the chosen option through triggerLabel when one is given', () => {
    render(
      <Combobox
        options={OPTIONS}
        value="UY"
        onValueChange={() => {}}
        placeholder="País"
        triggerLabel={(option) => `+${option.label.match(/\+(\d+)/)?.[1]}`}
      />,
    );

    expect(screen.getByRole('combobox').textContent).toBe('+598');
  });

  // A filter sitting on its reset option reads as unset, whatever triggerLabel would make of it.
  it('keeps the placeholder while the reset option is chosen', () => {
    render(
      <Combobox
        options={OPTIONS}
        value="AR"
        resetValue="AR"
        onValueChange={() => {}}
        placeholder="País"
        triggerLabel={() => 'nunca'}
      />,
    );

    expect(screen.getByRole('combobox').textContent).toBe('País');
  });

  // A reset alone in the list ("Todas" with nothing to filter by) gets no line under it.
  it('draws the line under the reset option only when other options follow', () => {
    const view = render(
      <Combobox
        options={[{ value: 'ALL', label: 'Todas' }]}
        value="ALL"
        resetValue="ALL"
        onValueChange={() => {}}
        placeholder="Sucursal"
      />,
    );
    fireEvent.click(screen.getByRole('combobox'));
    expect(document.querySelector('[data-slot="command-separator"], [cmdk-separator]')).toBeNull();
    view.unmount();

    render(
      <Combobox
        options={[{ value: 'ALL', label: 'Todas' }, ...OPTIONS]}
        value="ALL"
        resetValue="ALL"
        onValueChange={() => {}}
        placeholder="Sucursal"
      />,
    );
    fireEvent.click(screen.getByRole('combobox'));
    expect(
      document.querySelector('[data-slot="command-separator"], [cmdk-separator]'),
    ).not.toBeNull();
  });
});
