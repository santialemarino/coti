import { useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { browserEdit, typeText } from '@repo/vitest-config/input-edits';
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
        landlineNotice={{ message: 'Parece un fijo.', action: (mobile) => `Usar ${mobile}` }}
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

function undo() {
  const event = createEvent.keyDown(national(), { key: 'z', metaKey: true });
  fireEvent(national(), event);
  return event;
}

function start() {
  national().focus();
  national().setSelectionRange(0, 0);
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

  describe('an Argentine number without a 9 or a 15', () => {
    it('keeps the landline it reads as and offers the mobile instead of applying it', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);

      fireEvent.change(national(), { target: { value: '11 5555-0101' } });

      expect(onValue).toHaveBeenLastCalledWith('+541155550101');
      expect(screen.getByRole('status').textContent).toContain('Parece un fijo.');
    });

    it('switches to the mobile when the seller takes the offer', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      fireEvent.change(national(), { target: { value: '11 5555-0101' } });

      fireEvent.click(screen.getByRole('button', { name: 'Usar +54 9 11 5555 0101' }));

      expect(onValue).toHaveBeenLastCalledWith('+5491155550101');
      expect(national().value).toBe('91155550101');
      expect(screen.getByRole('status').textContent).toBe('');
    });

    it('offers nothing once the number is already a mobile', () => {
      render(<Harness onValue={vi.fn()} />);

      fireEvent.change(national(), { target: { value: '351 15 555 0101' } });

      expect(screen.getByRole('status').textContent).toBe('');
    });

    it('offers nothing outside Argentina, where the 9 means nothing', () => {
      render(<Harness onValue={vi.fn()} />);

      fireEvent.change(national(), { target: { value: '+598 423 1234' } });

      expect(screen.getByRole('status').textContent).toBe('');
    });

    it('offers nothing for a number that is not valid either way', () => {
      render(<Harness onValue={vi.fn()} />);

      fireEvent.change(national(), { target: { value: '15 5555-0101' } });

      expect(screen.getByRole('status').textContent).toBe('');
    });
  });

  describe('what the field shows', () => {
    it('settles a valid number to its bare digits when the seller leaves the field', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);

      fireEvent.change(national(), { target: { value: '(011) 15-5555.0101' } });
      fireEvent.blur(national());

      expect(national().value).toBe('91155550101');
      expect(onValue).toHaveBeenLastCalledWith('+5491155550101');
    });

    it('leaves a number it cannot read as the seller wrote it', () => {
      render(<Harness onValue={vi.fn()} />);

      fireEvent.change(national(), { target: { value: '15 5555' } });
      fireEvent.blur(national());

      expect(national().value).toBe('15 5555');
    });

    it('drops anything that cannot be part of a phone number', () => {
      render(<Harness onValue={vi.fn()} />);

      fireEvent.change(national(), { target: { value: '11 ab5555-0101x' } });

      expect(national().value).toBe('11 5555-0101');
    });
  });

  describe('undo', () => {
    it('keeps the caret in place when a typed letter is dropped', () => {
      render(<Harness onValue={vi.fn()} />);
      start();
      typeText(national(), '1155');

      browserEdit(national(), '11x55', { caret: 3 });

      expect(national().value).toBe('1155');
      expect([national().selectionStart, national().selectionEnd]).toEqual([2, 2]);
    });

    it('undoes a run of typing in one step', () => {
      render(<Harness onValue={vi.fn()} />);
      start();
      typeText(national(), '11 5555');

      expect(undo().defaultPrevented).toBe(true);

      expect(national().value).toBe('');
    });

    it('undoes a pasted international number, country included', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      start();
      typeText(national(), '11');

      national().setSelectionRange(0, 2);
      browserEdit(national(), '+598 99 123 456', { inputType: 'insertFromPaste' });
      expect(country().textContent).toContain('+598');

      undo();

      expect(country().textContent).toBe('🇦🇷+54');
      expect(national().value).toBe('11');
      expect(onValue).toHaveBeenLastCalledWith('+5411');
    });

    it('undoes the settling on blur back to the number as written', () => {
      render(<Harness onValue={vi.fn()} />);
      start();
      typeText(national(), '11 5555-0101');
      fireEvent.blur(national());
      expect(national().value).toBe('1155550101');

      undo();

      expect(national().value).toBe('11 5555-0101');
    });

    it('takes its own step when the browser undoes without announcing it', () => {
      render(<Harness onValue={vi.fn()} />);
      start();
      typeText(national(), '1155');

      browserEdit(national(), '115', { inputType: 'historyUndo', announced: false });

      expect(national().value).toBe('');
    });
  });
});
