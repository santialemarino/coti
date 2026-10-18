'use client';

import { forwardRef, useState } from 'react';

import { useCaretPlacement } from '../hooks/use-caret-placement';
import {
  editKind,
  useInputHistory,
  type EditKind,
  type InputSnapshot,
} from '../hooks/use-input-history';
import {
  deleteAcrossSeparator,
  editTaxId,
  formatTaxId,
  pastedTaxId,
  TAX_ID_MAX_DIGITS,
  taxIdDigits,
  type TaxIdEdit,
  type TaxIdKind,
} from '../lib/tax-id';
import { Input } from './input';

interface TaxIdInputProps {
  kind: TaxIdKind;
  /* Digits only — what the API stores. The field shows them formatted. */
  value?: string;
  onChange?: (digits: string) => void;
  onBlur?: () => void;
  name?: string;
  placeholder?: string;
  disabled?: boolean;
  autoFocus?: boolean;
  className?: string;
  containerClassName?: string;
  'aria-label'?: string;
  'aria-invalid'?: boolean | 'true' | 'false';
  'aria-describedby'?: string;
  id?: string;
}

// A CUIT, a DNI or either, formatted as typed; whatever cannot belong to the number is dropped, and
// form state only ever holds the digits.
const TaxIdInput = forwardRef<HTMLInputElement, TaxIdInputProps>(
  ({ kind, value = '', onChange, ...rest }, ref) => {
    const placeCaret = useCaretPlacement();
    // What the field shows while it is more than the formatted digits: a separator typed at the end.
    const [shown, setShown] = useState<string | null>(null);
    const digits = taxIdDigits(value);
    const display =
      shown !== null && taxIdDigits(shown) === digits ? shown : formatTaxId(digits, kind);
    const history = useInputHistory({
      ref,
      value: display,
      restore: (snapshot, input) => show(input, snapshot),
      onReset: () => setShown(null),
    });

    function show(input: HTMLInputElement, to: InputSnapshot) {
      const next = taxIdDigits(to.value);
      setShown(to.value);
      placeCaret(input, to.start, to.end);
      if (next !== digits) onChange?.(next);
    }

    function apply(
      input: HTMLInputElement,
      edited: EditKind,
      edit: TaxIdEdit,
      from: InputSnapshot,
    ) {
      const to = { value: edit.display, start: edit.caret, end: edit.caret };
      history.record(edited, from, to);
      show(input, to);
    }

    return (
      <Input
        {...rest}
        ref={history.inputRef}
        type="text"
        inputMode="numeric"
        autoComplete="off"
        value={display}
        onChange={(event) => {
          if (history.replay(event)) return;
          const input = event.currentTarget;
          const caret = input.selectionStart ?? input.value.length;
          const edit = editTaxId(input.value, caret, display, kind);
          apply(input, editKind(event.nativeEvent), edit, history.before());
        }}
        onKeyDown={(event) => {
          if (history.onKeyDown(event)) return;
          const input = event.currentTarget;
          // A modified Backspace deletes a word or a line, which the browser does as usual.
          if (event.altKey || event.metaKey || event.ctrlKey) return;
          if (input.selectionStart === null || input.selectionStart !== input.selectionEnd) return;
          const edit = deleteAcrossSeparator(display, input.selectionStart, event.key, kind);
          if (!edit) return;
          event.preventDefault();
          apply(input, 'delete', edit, history.snapshot(input));
        }}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const from = history.snapshot(input);
          // A paste fills the room the field has left; a typed digit past it is refused instead.
          const selected = taxIdDigits(input.value.slice(from.start, from.end)).length;
          const room = Math.max(0, TAX_ID_MAX_DIGITS[kind] - digits.length + selected);
          const pasted = pastedTaxId(event.clipboardData.getData('text/plain'), room);
          const raw = input.value.slice(0, from.start) + pasted + input.value.slice(from.end);
          const edit = editTaxId(raw, from.start + pasted.length, display, kind);
          apply(input, 'other', edit, from);
        }}
      />
    );
  },
);

TaxIdInput.displayName = 'TaxIdInput';

export { TaxIdInput };
