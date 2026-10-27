import { useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { browserEdit, typeText } from '@repo/vitest-config/input-edits';
import { QuantityInput } from '@/components/quantity-input';

function Harness({
  initial = '',
  maxLength,
  onValue = vi.fn(),
}: {
  initial?: string;
  maxLength?: number;
  onValue?: (value: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <QuantityInput
        aria-label="Días"
        maxLength={maxLength}
        value={value}
        onChange={(next) => {
          setValue(next);
          onValue(next);
        }}
      />
      <button type="button" onClick={() => setValue('30')}>
        reset
      </button>
    </>
  );
}

function input() {
  return screen.getByRole('textbox', { name: 'Días' }) as HTMLInputElement;
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

function selection() {
  return [input().selectionStart, input().selectionEnd];
}

describe('QuantityInput', () => {
  it('keeps the caret in place when a typed letter is dropped', () => {
    const onValue = vi.fn();
    render(<Harness initial="1234" onValue={onValue} />);

    browserEdit(input(), '12x34', { caret: 3 });

    expect(input().value).toBe('1234');
    expect(selection()).toEqual([2, 2]);
    expect(onValue).not.toHaveBeenCalled();
  });

  it('pastes at the caret and leaves the caret after what was pasted', () => {
    render(<Harness initial="1234" />);
    select(2);

    paste('9 9');

    expect(input().value).toBe('129934');
    expect(selection()).toEqual([4, 4]);
  });

  it('fills only the room maxLength leaves with a paste, as typing would', () => {
    const onValue = vi.fn();
    render(<Harness initial="12" maxLength={4} onValue={onValue} />);
    select(2);

    paste('3456');

    expect(input().value).toBe('1234');
    expect(onValue).toHaveBeenLastCalledWith('1234');
  });

  it('undoes a run of typing in one step', () => {
    render(<Harness />);
    select(0);
    typeText(input(), '123');

    expect(undo().defaultPrevented).toBe(true);

    expect(input().value).toBe('');
  });

  it('undoes a paste on its own, putting the caret back', () => {
    render(<Harness />);
    select(0);
    typeText(input(), '12');
    paste('34');

    undo();

    expect(input().value).toBe('12');
    expect(selection()).toEqual([2, 2]);
  });

  it('undoes an arrow step', () => {
    render(<Harness initial="5" />);
    select(1);

    press('ArrowUp');
    expect(input().value).toBe('6');
    undo();

    expect(input().value).toBe('5');
  });

  it('takes its own step when the browser undoes without announcing it', () => {
    render(<Harness />);
    select(0);
    typeText(input(), '12');
    paste('34');

    browserEdit(input(), '123', { inputType: 'historyUndo', announced: false });

    expect(input().value).toBe('12');
  });

  it('leaves shortcuts to the browser', () => {
    render(<Harness />);
    select(0);

    expect(press('-', { metaKey: true }).defaultPrevented).toBe(false);
    expect(press('-').defaultPrevented).toBe(true);
  });

  it('starts over when the value changes from outside', () => {
    render(<Harness />);
    select(0);
    typeText(input(), '7');
    fireEvent.click(screen.getByRole('button', { name: 'reset' }));

    undo();

    expect(input().value).toBe('30');
  });
});
