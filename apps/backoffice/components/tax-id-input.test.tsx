import { useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { TaxIdInput } from '@/components/tax-id-input';
import type { TaxIdKind } from '@/lib/utils/tax-id';

function Harness({
  kind,
  initial = '',
  onValue = vi.fn(),
}: {
  kind: TaxIdKind;
  initial?: string;
  onValue?: (value: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <TaxIdInput
      kind={kind}
      aria-label="CUIT"
      value={value}
      onChange={(next) => {
        setValue(next);
        onValue(next);
      }}
    />
  );
}

function input() {
  return screen.getByRole('textbox', { name: 'CUIT' }) as HTMLInputElement;
}

const setNativeValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;

// What the browser hands React after an edit: the raw text, with the caret where the edit left it.
function edit(raw: string, caret = raw.length) {
  const field = input();
  setNativeValue.call(field, raw);
  field.setSelectionRange(caret, caret);
  fireEvent.input(field);
}

// Each keystroke at the end, the way someone types the number.
function type(text: string) {
  [...text].forEach((char) => edit(input().value + char));
}

function placeCaret(at: number, end = at) {
  input().focus();
  input().setSelectionRange(at, end);
}

function paste(text: string) {
  const field = input();
  const event = createEvent.paste(field, { clipboardData: { getData: () => text } });
  fireEvent(field, event);
}

function press(key: string) {
  fireEvent.keyDown(input(), { key });
}

describe('TaxIdInput', () => {
  it('formats a CUIT as it is typed and holds only its digits', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit" onValue={onValue} />);

    type('20');
    expect(input().value).toBe('20');
    type('1');
    expect(input().value).toBe('20-1');
    type('23456789');

    expect(input().value).toBe('20-12345678-9');
    expect(onValue).toHaveBeenLastCalledWith('20123456789');
  });

  it('groups a DNI in thousands as it is typed', () => {
    render(<Harness kind="dni" />);

    type('1234');
    expect(input().value).toBe('1.234');
    type('5678');

    expect(input().value).toBe('12.345.678');
  });

  it('is a numeric field the browser does not autofill', () => {
    render(<Harness kind="cuit" />);

    expect(input().getAttribute('inputmode')).toBe('numeric');
    expect(input().getAttribute('autocomplete')).toBe('off');
    expect(input().getAttribute('type')).toBe('text');
  });

  it('ignores letters and symbols typed among the digits', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit" onValue={onValue} />);

    type('2a0-b1/');

    expect(input().value).toBe('20-1');
    expect(onValue).toHaveBeenLastCalledWith('201');
  });

  it('keeps the caret in place when a typed letter is dropped', () => {
    render(<Harness kind="cuit" initial="20123456789" />);

    // "x" typed between the 4 and the 5 of 20-1234|5678-9.
    edit('20-1234x5678-9', 8);

    expect(input().value).toBe('20-12345678-9');
    expect(input().selectionStart).toBe(7);
  });

  it.each([['20123456789'], ['20.12345678.9'], ['20 12345678 9'], ['  20-12345678-9  ']])(
    'reformats the pasted %p',
    (pasted) => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);
      placeCaret(0);

      paste(pasted);

      expect(input().value).toBe('20-12345678-9');
      expect(input().selectionStart).toBe(13);
      expect(onValue).toHaveBeenLastCalledWith('20123456789');
    },
  );

  it('caps a CUIT at eleven digits, keeping the first ones of an overflowing paste', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit" onValue={onValue} />);
    placeCaret(0);

    paste('2012345678999');

    expect(input().value).toBe('20-12345678-9');
    expect(onValue).toHaveBeenLastCalledWith('20123456789');
  });

  it('refuses a digit typed into a full field instead of pushing the last one out', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit" initial="20123456789" onValue={onValue} />);

    // "5" typed after the 20- prefix.
    edit('20-512345678-9', 4);

    expect(input().value).toBe('20-12345678-9');
    expect(input().selectionStart).toBe(3);
    expect(onValue).not.toHaveBeenCalled();
  });

  it('caps a DNI at eight digits', () => {
    render(<Harness kind="dni" />);

    type('123456789');

    expect(input().value).toBe('12.345.678');
  });

  it('takes the digit before a separator on backspace, instead of the separator alone', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit" initial="20123456789" onValue={onValue} />);
    placeCaret(3);

    press('Backspace');

    expect(input().value).toBe('21-23456789');
    expect(input().selectionStart).toBe(1);
    expect(onValue).toHaveBeenLastCalledWith('2123456789');
  });

  it('takes the digit after a separator on delete', () => {
    render(<Harness kind="cuit" initial="20123456789" />);
    placeCaret(2);

    press('Delete');

    expect(input().value).toBe('20-23456789');
    expect(input().selectionStart).toBe(2);
  });

  it('leaves a backspace over a digit to the browser', () => {
    render(<Harness kind="cuit" initial="20123456789" />);
    placeCaret(5);

    const event = createEvent.keyDown(input(), { key: 'Backspace' });
    fireEvent(input(), event);

    expect(event.defaultPrevented).toBe(false);
  });

  // Separators move as the middle of the number changes; the caret follows the digits instead.
  it('keeps the caret after the digit typed in the middle', () => {
    render(<Harness kind="dni" initial="1234567" />);

    // "9" typed after the 1 of 1.|234.567.
    edit('1.9234.567', 3);

    expect(input().value).toBe('19.234.567');
    expect(input().selectionStart).toBe(2);
  });

  it('keeps the caret where a digit was deleted from the middle', () => {
    render(<Harness kind="dni" initial="12345678" />);

    // The 3 of 12.3|45.678 deleted.
    edit('12.45.678', 3);

    expect(input().value).toBe('1.245.678');
    expect(input().selectionStart).toBe(3);
  });

  it('switches a CUIT or DNI from the DNI shape to the CUIT one at the ninth digit', () => {
    const onValue = vi.fn();
    render(<Harness kind="cuit-or-dni" onValue={onValue} />);

    type('20123456');
    expect(input().value).toBe('20.123.456');
    type('7');
    expect(input().value).toBe('20-1234567');
    expect(input().selectionStart).toBe(10);
    type('89');
    expect(input().value).toBe('20-12345678-9');

    edit('20-12345678-', 12);
    edit('20-1234567', 10);
    edit('20-123456', 9);
    expect(input().value).toBe('20.123.456');
    expect(onValue).toHaveBeenLastCalledWith('20123456');
  });

  it('shows a value with separators formatted for its kind', () => {
    render(<Harness kind="cuit-or-dni" initial="30-70123456-8" />);

    expect(input().value).toBe('30-70123456-8');
  });
});
