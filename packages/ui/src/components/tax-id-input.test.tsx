import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { TaxIdInput } from './tax-id-input';

describe('TaxIdInput read-only identities', () => {
  it('preserves the authenticated tax ID through typing, separator deletion and paste', () => {
    const change = vi.fn();
    render(
      <TaxIdInput
        kind="cuit"
        value="20123456786"
        readOnly
        autoComplete="username"
        onChange={change}
        aria-label="Tax ID"
      />,
    );
    const input = screen.getByLabelText('Tax ID') as HTMLInputElement;
    expect(input.readOnly).toBe(true);
    expect(input.autocomplete).toBe('username');
    input.setSelectionRange(3, 3);
    fireEvent.keyDown(input, { key: 'Backspace' });
    fireEvent.paste(input, { clipboardData: { getData: () => '20999999999' } });
    fireEvent.change(input, { target: { value: '20999999999' } });
    expect(change).not.toHaveBeenCalled();
  });
});
