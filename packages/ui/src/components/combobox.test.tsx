import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Combobox } from './combobox';

const OPTIONS = [
  { value: 'AR', label: 'Argentina (+54)' },
  { value: 'UY', label: 'Uruguay (+598)' },
];

describe('Combobox', () => {
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
});
