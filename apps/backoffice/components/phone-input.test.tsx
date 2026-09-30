import { useState } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { PhoneInput } from '@/components/phone-input';
import messages from '@/translations/es.json';

function Harness({
  initial = '',
  onValue,
}: {
  initial?: string;
  onValue: (value: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <NextIntlClientProvider locale="es" messages={messages}>
      <PhoneInput
        id="phone"
        value={value}
        onChange={(next) => {
          setValue(next);
          onValue(next);
        }}
        countryLabel="País"
        countrySearchPlaceholder="Buscá un país"
        countryEmptyLabel="Nada"
      />
      <button type="button" onClick={() => setValue('')}>
        Vaciar
      </button>
    </NextIntlClientProvider>
  );
}

function national() {
  return screen.getByRole('textbox') as HTMLInputElement;
}

function country() {
  return screen.getByRole('combobox', { name: 'País' });
}

describe('PhoneInput', () => {
  it('reads a national number in Argentina by default and emits it as E.164', () => {
    const onValue = vi.fn();
    render(<Harness onValue={onValue} />);

    fireEvent.change(national(), { target: { value: '11 15 5555-0101' } });

    expect(country().textContent).toBe('🇦🇷+54');
    expect(onValue).toHaveBeenLastCalledWith('+5491155550101');
  });

  // A seller pasting a client's full international number should not have to split it by hand.
  it('moves to the country of a pasted international number', () => {
    const onValue = vi.fn();
    render(<Harness onValue={onValue} />);

    fireEvent.change(national(), { target: { value: '+598 99 123 456' } });

    expect(country().textContent).toContain('+598');
    expect(national().value).toBe('99123456');
    expect(onValue).toHaveBeenLastCalledWith('+59899123456');
  });

  it('splits a prefilled E.164 value into its country and national number', () => {
    render(<Harness initial="+5493515550101" onValue={vi.fn()} />);

    expect(country().textContent).toContain('+54');
    expect(national().value).toBe('93515550101');
  });

  it('takes a value the caller resets, rather than keeping what was typed', () => {
    render(<Harness onValue={vi.fn()} />);
    fireEvent.change(national(), { target: { value: '11 15 5555-0101' } });

    fireEvent.click(screen.getByRole('button', { name: 'Vaciar' }));

    expect(national().value).toBe('');
  });
});
