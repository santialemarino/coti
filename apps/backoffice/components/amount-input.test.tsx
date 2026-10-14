import { useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { beforeInput, browserEdit, typeText } from '@repo/vitest-config/input-edits';
import { AmountInput } from '@/components/amount-input';

function Harness({
  initial = '',
  maxDecimals = 2,
  onValue = vi.fn(),
}: {
  initial?: string;
  maxDecimals?: number;
  onValue?: (value: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <NextIntlClientProvider locale="es" messages={{}}>
      <AmountInput
        aria-label="Precio"
        maxDecimals={maxDecimals}
        value={value}
        onChange={(next) => {
          setValue(next);
          onValue(next);
        }}
      />
      <button type="button" onClick={() => setValue('500')}>
        reset
      </button>
    </NextIntlClientProvider>
  );
}

function input() {
  return screen.getByRole('textbox', { name: 'Precio' }) as HTMLInputElement;
}

function type(text: string) {
  typeText(input(), text);
}

function select(start: number, end = start) {
  input().focus();
  input().setSelectionRange(start, end);
}

function paste(text: string) {
  const field = input();
  fireEvent(field, createEvent.paste(field, { clipboardData: { getData: () => text } }));
}

function press(key: string, modifiers: { metaKey?: boolean; shiftKey?: boolean } = {}) {
  const event = createEvent.keyDown(input(), { key, ...modifiers });
  fireEvent(input(), event);
  return event;
}

const undo = () => press('z', { metaKey: true });
const redo = () => press('z', { metaKey: true, shiftKey: true });

function selection() {
  return [input().selectionStart, input().selectionEnd];
}

describe('AmountInput', () => {
  describe('formatting', () => {
    it('groups thousands as typed and holds the canonical value', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      select(0);

      type('1234,5');

      expect(input().value).toBe('1.234,5');
      expect(onValue).toHaveBeenLastCalledWith('1234.5');
    });

    it('keeps the caret in place when a typed letter is dropped', () => {
      render(<Harness initial="1234" />);

      // "x" typed between the 2 and the 3 of 1.2|34.
      browserEdit(input(), '1.2x34', { caret: 4 });

      expect(input().value).toBe('1.234');
      expect(selection()).toEqual([3, 3]);
    });

    it('refuses a fraction digit typed into a full fraction instead of pushing one out', () => {
      const onValue = vi.fn();
      render(<Harness initial="1.23" onValue={onValue} />);

      // "9" typed after the comma of 1,|23.
      browserEdit(input(), '1,923', { caret: 3 });

      expect(input().value).toBe('1,23');
      expect(selection()).toEqual([2, 2]);
      expect(onValue).not.toHaveBeenCalled();
    });

    it('leaves shortcuts to the browser while refusing the characters they share a key with', () => {
      render(<Harness />);
      select(0);

      expect(press('-', { metaKey: true }).defaultPrevented).toBe(false);
      expect(press('e', { metaKey: true }).defaultPrevented).toBe(false);
      expect(press('-').defaultPrevented).toBe(true);
      expect(press('e').defaultPrevented).toBe(true);
    });
  });

  describe('typed separators', () => {
    it('ends an amount typed with its separators exactly as typed', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      select(0);

      type('1.234.567,89');

      expect(input().value).toBe('1.234.567,89');
      expect(onValue).toHaveBeenLastCalledWith('1234567.89');
    });

    it('shows a group separator typed at the end of the whole part until a digit regroups it', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      select(0);

      type('1.');
      expect(input().value).toBe('1.');
      expect(selection()).toEqual([2, 2]);
      expect(onValue).toHaveBeenLastCalledWith('1');

      type('2');
      expect(input().value).toBe('12');
    });

    it.each([
      ['the decimal separator', '12.5', 2, ',', '12,5', 3],
      ['a group separator', '1234', 1, '.', '1.234', 2],
    ])('steps over %s right after the caret', (_, initial, caret, typed, shown, after) => {
      const onValue = vi.fn();
      render(<Harness initial={initial} onValue={onValue} />);
      select(caret);

      type(typed);

      expect(input().value).toBe(shown);
      expect(selection()).toEqual([after, after]);
      expect(onValue).not.toHaveBeenCalled();
    });

    it.each([
      ['a second decimal separator', '12.5', 4, ','],
      ['a group separator in the fraction', '12.5', 4, '.'],
      ['a group separator inside the number', '1234', 3, '.'],
    ])('ignores %s', (_, initial, caret, typed) => {
      render(<Harness initial={initial} />);
      const shown = input().value;
      select(caret);

      type(typed);

      expect(input().value).toBe(shown);
      expect(selection()).toEqual([caret, caret]);
    });
  });

  describe('undo and redo', () => {
    it('undoes a run of typing in one step, and redoes it', () => {
      render(<Harness />);
      select(0);
      type('1234567');
      expect(input().value).toBe('1.234.567');

      expect(undo().defaultPrevented).toBe(true);
      expect(input().value).toBe('');

      redo();
      expect(input().value).toBe('1.234.567');
      expect(selection()).toEqual([9, 9]);
    });

    it('undoes a paste on its own, selecting again what it replaced', () => {
      const onValue = vi.fn();
      render(<Harness initial="1234" onValue={onValue} />);
      select(0, 5);
      paste('1,234.56');
      expect(input().value).toBe('1.234,56');

      undo();

      expect(input().value).toBe('1.234');
      expect(selection()).toEqual([0, 5]);
      expect(onValue).toHaveBeenLastCalledWith('1234');
    });

    it('undoes an arrow step', () => {
      render(<Harness />);
      select(0);
      type('5');

      press('ArrowUp');
      expect(input().value).toBe('6');
      undo();

      expect(input().value).toBe('5');
    });

    it('answers the Edit menu, cancelling the browser’s own history', () => {
      render(<Harness />);
      select(0);
      type('1234');

      expect(beforeInput(input(), 'historyUndo')).toBe(true);

      expect(input().value).toBe('');
    });

    it('takes its own step when the browser undoes without announcing it', () => {
      const onValue = vi.fn();
      render(<Harness onValue={onValue} />);
      select(0);
      type('1234');

      // Chromium's scripted undo reverts its own stack, which only holds the last keystroke.
      browserEdit(input(), '1.23', { inputType: 'historyUndo', announced: false });
      expect(input().value).toBe('');
      expect(onValue).toHaveBeenLastCalledWith('');

      browserEdit(input(), '1.23', { inputType: 'historyRedo', announced: false });
      expect(input().value).toBe('1.234');
    });

    it('starts over when the value changes from outside', () => {
      render(<Harness />);
      select(0);
      type('12');
      fireEvent.click(screen.getByRole('button', { name: 'reset' }));
      expect(input().value).toBe('500');

      undo();

      expect(input().value).toBe('500');
    });
  });
});
