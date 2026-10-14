import { createRef, useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { beforeInput, browserEdit, typeText } from '@repo/vitest-config/input-edits';
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
    <>
      <TaxIdInput
        kind={kind}
        aria-label="CUIT"
        value={value}
        onChange={(next) => {
          setValue(next);
          onValue(next);
        }}
      />
      <button type="button" onClick={() => setValue('30701234568')}>
        reset
      </button>
    </>
  );
}

interface Modifiers {
  metaKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
}

function input() {
  return screen.getByRole('textbox', { name: 'CUIT' }) as HTMLInputElement;
}

function type(text: string) {
  typeText(input(), text);
}

// What the browser leaves after a deletion it performs itself.
function erase(raw: string, caret: number, inputType = 'deleteContentBackward') {
  browserEdit(input(), raw, { caret, inputType });
}

function select(start: number, end = start) {
  input().focus();
  input().setSelectionRange(start, end);
}

function paste(text: string) {
  const field = input();
  fireEvent(field, createEvent.paste(field, { clipboardData: { getData: () => text } }));
}

function press(key: string, modifiers: Modifiers = {}) {
  const event = createEvent.keyDown(input(), { key, ...modifiers });
  fireEvent(input(), event);
  return event;
}

const undo = () => press('z', { metaKey: true });
const redo = () => press('z', { metaKey: true, shiftKey: true });

const nextTask = () => new Promise((resolve) => setTimeout(resolve));

function selection() {
  return [input().selectionStart, input().selectionEnd];
}

afterEach(() => {
  vi.useRealTimers();
});

describe('TaxIdInput', () => {
  describe('formatting', () => {
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

    it('is a numeric field the browser does not autofill, capped by digits and not maxLength', () => {
      render(<Harness kind="cuit" />);

      expect(input().getAttribute('inputmode')).toBe('numeric');
      expect(input().getAttribute('autocomplete')).toBe('off');
      expect(input().getAttribute('type')).toBe('text');
      expect(input().hasAttribute('maxlength')).toBe(false);
    });

    it('ignores letters and symbols typed among the digits', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);

      type('2a0b1/');

      expect(input().value).toBe('20-1');
      expect(onValue).toHaveBeenLastCalledWith('201');
    });

    it('keeps the caret in place when a typed letter is dropped', () => {
      render(<Harness kind="cuit" initial="20123456789" />);

      // "x" typed between the 4 and the 5 of 20-1234|5678-9.
      browserEdit(input(), '20-1234x5678-9', { caret: 8 });

      expect(input().value).toBe('20-12345678-9');
      expect(input().selectionStart).toBe(7);
    });

    it.each([['20123456789'], ['20.12345678.9'], ['20 12345678 9'], ['  20-12345678-9  ']])(
      'reformats the pasted %p',
      (pasted) => {
        const onValue = vi.fn();
        render(<Harness kind="cuit" onValue={onValue} />);
        select(0);

        paste(pasted);

        expect(input().value).toBe('20-12345678-9');
        expect(input().selectionStart).toBe(13);
        expect(onValue).toHaveBeenLastCalledWith('20123456789');
      },
    );

    it('caps a CUIT at eleven digits, keeping the first ones of an overflowing paste', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);
      select(0);

      paste('2012345678999');

      expect(input().value).toBe('20-12345678-9');
      expect(onValue).toHaveBeenLastCalledWith('20123456789');
    });

    it('refuses a digit typed into a full field instead of pushing the last one out', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" initial="20123456789" onValue={onValue} />);

      // "5" typed after the 20- prefix.
      browserEdit(input(), '20-512345678-9', { caret: 4 });

      expect(input().value).toBe('20-12345678-9');
      expect(input().selectionStart).toBe(3);
      expect(onValue).not.toHaveBeenCalled();
    });

    it('caps a DNI at eight digits', () => {
      render(<Harness kind="dni" />);

      type('123456789');

      expect(input().value).toBe('12.345.678');
    });

    // Separators move as the middle of the number changes; the caret follows the digits instead.
    it('keeps the caret after the digit typed in the middle', () => {
      render(<Harness kind="dni" initial="1234567" />);

      // "9" typed after the 1 of 1.|234.567.
      browserEdit(input(), '1.9234.567', { caret: 3 });

      expect(input().value).toBe('19.234.567');
      expect(input().selectionStart).toBe(2);
    });

    it('keeps the caret where a digit was deleted from the middle', () => {
      render(<Harness kind="dni" initial="12345678" />);

      // The 3 of 12.3|45.678 deleted.
      erase('12.45.678', 3);

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

      erase('20-12345678-', 12);
      erase('20-1234567', 10);
      erase('20-123456', 9);
      expect(input().value).toBe('20.123.456');
      expect(onValue).toHaveBeenLastCalledWith('20123456');
    });

    it('shows a value with separators formatted for its kind', () => {
      render(<Harness kind="cuit-or-dni" initial="30-70123456-8" />);

      expect(input().value).toBe('30-70123456-8');
    });
  });

  describe('typed separators', () => {
    it('shows a separator typed where the next one goes, while the form holds only digits', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);

      type('20-');

      expect(input().value).toBe('20-');
      expect(selection()).toEqual([3, 3]);
      expect(onValue).toHaveBeenLastCalledWith('20');

      type('1');
      expect(input().value).toBe('20-1');
      expect(selection()).toEqual([4, 4]);
    });

    it.each<[TaxIdKind, string]>([
      ['cuit', '20-12345678-9'],
      ['cuit-or-dni', '20-12345678-9'],
      ['dni', '12.345.678'],
      ['dni', '1.234.567'],
      ['cuit-or-dni', '12.345.678'],
    ])('ends a %s typed with its separators exactly as typed: %s', (kind, typed) => {
      render(<Harness kind={kind} />);

      type(typed);

      expect(input().value).toBe(typed);
      expect(input().selectionStart).toBe(typed.length);
    });

    it('steps over the literal right after the caret instead of doubling it', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" initial="2012345" onValue={onValue} />);
      select(2);

      type('-');

      expect(input().value).toBe('20-12345');
      expect(selection()).toEqual([3, 3]);
      expect(onValue).not.toHaveBeenCalled();
    });

    it.each<[TaxIdKind, string, string, string]>([
      ['cuit', '20-1', '-', '20-1'],
      ['cuit', '20', '.', '20'],
      ['cuit', '2', '-', '2'],
      ['dni', '123', '.', '123'],
      // A CUIT or DNI is DNI-shaped until its ninth digit, so its hyphen waits for the tenth.
      ['cuit-or-dni', '20', '-', '20'],
    ])('ignores a separator anywhere else: %s %p + %p', (kind, typed, separator, shown) => {
      render(<Harness kind={kind} />);
      type(typed);

      type(separator);

      expect(input().value).toBe(shown);
      expect(input().selectionStart).toBe(shown.length);
    });

    it('lets backspace take a separator at the end alone', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);
      type('20-1');

      erase('20-', 3);
      expect(input().value).toBe('20-');

      expect(press('Backspace').defaultPrevented).toBe(false);
      erase('20', 2);
      expect(input().value).toBe('20');
      expect(onValue).toHaveBeenLastCalledWith('20');
    });

    it('lets delete take a separator at the end alone', () => {
      render(<Harness kind="cuit" />);
      type('20-');
      select(2);

      expect(press('Delete').defaultPrevented).toBe(false);
      erase('20', 2, 'deleteContentForward');

      expect(input().value).toBe('20');
      expect(selection()).toEqual([2, 2]);
    });

    it('takes the digit before a separator inside the number on backspace', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" initial="20123456789" onValue={onValue} />);
      select(3);

      expect(press('Backspace').defaultPrevented).toBe(true);

      expect(input().value).toBe('21-23456789');
      expect(input().selectionStart).toBe(1);
      expect(onValue).toHaveBeenLastCalledWith('2123456789');
    });

    it('takes the digit after a separator inside the number on delete', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(2);

      press('Delete');

      expect(input().value).toBe('20-23456789');
      expect(input().selectionStart).toBe(2);
    });

    it('leaves a backspace over a digit, and a modified one, to the browser', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(5);
      expect(press('Backspace').defaultPrevented).toBe(false);

      select(3);
      expect(press('Backspace', { metaKey: true }).defaultPrevented).toBe(false);
    });
  });

  describe('undo and redo', () => {
    it('undoes a run of typing in one step, and redoes it', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('2032');
      expect(input().value).toBe('20-32');

      expect(undo().defaultPrevented).toBe(true);
      expect(input().value).toBe('');
      expect(selection()).toEqual([0, 0]);

      redo();
      expect(input().value).toBe('20-32');
      expect(selection()).toEqual([5, 5]);
    });

    it('keeps typed separators inside the run they were typed in', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);
      select(0);
      type('20-12345678-9');

      undo();

      expect(input().value).toBe('');
      expect(onValue).toHaveBeenLastCalledWith('');
    });

    it('closes the run after a pause', () => {
      vi.useFakeTimers({ toFake: ['Date'] });
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      vi.setSystemTime(Date.now() + 2000);
      type('32');

      undo();
      expect(input().value).toBe('20');
      undo();
      expect(input().value).toBe('');
    });

    it('closes the run when the caret moves', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('2032');
      select(2);
      type('9');
      expect(input().value).toBe('20-932');

      undo();
      expect(input().value).toBe('20-32');
      expect(selection()).toEqual([2, 2]);
      undo();
      expect(input().value).toBe('');
    });

    it('makes a change of edit kind its own step, and a run of deletions one', () => {
      render(<Harness kind="dni" />);
      select(0);
      type('12345');
      erase('12.34', 5);
      erase('1.23', 4);
      expect(input().value).toBe('123');

      undo();
      expect(input().value).toBe('12.345');
      undo();
      expect(input().value).toBe('');
    });

    it('undoes a paste on its own, and the caret goes back', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      paste('12345678');
      expect(input().value).toBe('20-12345678');

      undo();
      expect(input().value).toBe('20');
      expect(selection()).toEqual([2, 2]);

      redo();
      expect(input().value).toBe('20-12345678');
    });

    it('selects again what a paste replaced', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(3, 11);
      paste('87654321');
      expect(input().value).toBe('20-87654321-9');

      undo();

      expect(input().value).toBe('20-12345678-9');
      expect(selection()).toEqual([3, 11]);
    });

    it('undoes a cut on its own', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('2012');
      select(3, 5);
      browserEdit(input(), '20-', { caret: 3, inputType: 'deleteByCut' });
      expect(input().value).toBe('20-');

      undo();
      expect(input().value).toBe('20-12');
      expect(selection()).toEqual([3, 5]);
      undo();
      expect(input().value).toBe('');
    });

    it('replaces a selection with what is typed, as one more step', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(0, input().value.length);

      type('27');
      expect(input().value).toBe('27');

      undo();
      expect(input().value).toBe('20-12345678-9');
      expect(selection()).toEqual([0, 13]);
    });

    it('takes a drop as one step, filtered like any edit', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      browserEdit(input(), '20 1234x', { caret: 8, inputType: 'insertFromDrop' });
      expect(input().value).toBe('20-1234');

      undo();
      expect(input().value).toBe('20');
    });

    it('takes autofill, which fires no beforeinput, as one step', () => {
      const onValue = vi.fn();
      render(<Harness kind="cuit" onValue={onValue} />);
      select(0);
      type('2');

      browserEdit(input(), '20-12345678-9', { inputType: null });
      expect(input().value).toBe('20-12345678-9');
      expect(onValue).toHaveBeenLastCalledWith('20123456789');

      undo();
      expect(input().value).toBe('2');
    });

    it('drops the redo steps when a new edit follows an undo', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      paste('1');
      undo();

      type('5');
      redo();

      expect(input().value).toBe('20-5');
    });

    it.each<[string, Modifiers, string, Modifiers]>([
      ['Ctrl+Z, Ctrl+Y', { ctrlKey: true }, 'y', {}],
      ['Ctrl+Z, Ctrl+Shift+Z', { ctrlKey: true }, 'z', { shiftKey: true }],
      ['⌘Z, ⌘⇧Z', { metaKey: true }, 'Z', { shiftKey: true }],
    ])('answers %s', (_, modifiers, redoKey, redoModifiers) => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');

      press('z', modifiers);
      expect(input().value).toBe('');
      press(redoKey, { ...modifiers, ...redoModifiers });
      expect(input().value).toBe('20');
    });

    it('answers the Edit menu, cancelling the browser’s own history', async () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      paste('32');

      expect(beforeInput(input(), 'historyUndo')).toBe(true);
      expect(input().value).toBe('20');
      await nextTask();
      expect(beforeInput(input(), 'historyRedo')).toBe(true);
      expect(input().value).toBe('20-32');
    });

    it('takes one step for the burst of history events WebKit fires for one menu undo', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      paste('32');

      expect([1, 2, 3].map(() => beforeInput(input(), 'historyUndo'))).toEqual([true, true, true]);

      expect(input().value).toBe('20');
    });

    it('starts over when the value changes from outside', () => {
      render(<Harness kind="cuit" />);
      select(0);
      type('20');
      fireEvent.click(screen.getByRole('button', { name: 'reset' }));
      expect(input().value).toBe('30-70123456-8');

      undo();

      expect(input().value).toBe('30-70123456-8');
    });

    it('does nothing, quietly, with nothing to undo', () => {
      render(<Harness kind="cuit" initial="20" />);

      expect(undo().defaultPrevented).toBe(true);
      expect(input().value).toBe('20');
    });
  });

  describe('as a native field', () => {
    it('leaves caret movement, selection and copying to the browser', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(5);

      ['ArrowLeft', 'ArrowRight', 'Home', 'End'].forEach((key) => {
        expect(press(key).defaultPrevented).toBe(false);
        expect(press(key, { shiftKey: true }).defaultPrevented).toBe(false);
      });
      expect(press('a', { metaKey: true }).defaultPrevented).toBe(false);
      expect(fireEvent.copy(input())).toBe(true);
    });

    it('replaces a shift-selected stretch with the digit typed', () => {
      render(<Harness kind="cuit" initial="20123456789" />);
      select(3, 7);

      type('9');

      expect(input().value).toBe('20-956789');
      expect(selection()).toEqual([4, 4]);
    });

    it('passes its accessible name, state and description through', () => {
      render(
        <TaxIdInput
          kind="cuit"
          value="20123456789"
          aria-label="CUIT"
          aria-invalid
          aria-describedby="cuit-hint"
          id="cuit"
        />,
      );

      expect(input().getAttribute('aria-invalid')).toBe('true');
      expect(input().getAttribute('aria-describedby')).toBe('cuit-hint');
      expect(input().id).toBe('cuit');
      expect(input().value).toBe('20-12345678-9');
    });

    it('forwards its ref to the input, so a form can focus it', () => {
      const objectRef = createRef<HTMLInputElement>();
      const callbackRef = vi.fn();
      const { unmount } = render(
        <>
          <TaxIdInput kind="cuit" aria-label="CUIT" ref={objectRef} />
          <TaxIdInput kind="dni" aria-label="DNI" ref={callbackRef} />
        </>,
      );

      objectRef.current?.focus();
      expect(document.activeElement).toBe(input());
      expect(callbackRef).toHaveBeenLastCalledWith(screen.getByRole('textbox', { name: 'DNI' }));

      unmount();
      expect(objectRef.current).toBeNull();
      expect(callbackRef).toHaveBeenLastCalledWith(null);
    });
  });
});
