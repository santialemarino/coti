'use client';

import { forwardRef } from 'react';

import { Input } from '@repo/ui/components';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import {
  deleteAcrossSeparator,
  editTaxId,
  formatTaxId,
  TAX_ID_MAX_DIGITS,
  taxIdDigits,
  type TaxIdEdit,
  type TaxIdKind,
} from '@/lib/utils/tax-id';

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

/*
 * A CUIT (`30-70123456-8`), a DNI (`12.345.678`) or either one, formatted as it is typed. Anything
 * that cannot belong to the number is dropped rather than refused, so a pasted `20.12345678.9` or
 * `20 12345678 9` lands as the same digits, and form state only ever holds those digits.
 */
const TaxIdInput = forwardRef<HTMLInputElement, TaxIdInputProps>(
  ({ kind, value = '', onChange, ...rest }, ref) => {
    const placeCaret = useCaretPlacement();
    const digits = taxIdDigits(value);
    const display = formatTaxId(digits, kind);

    function apply(input: HTMLInputElement, edit: TaxIdEdit) {
      placeCaret(input, edit.caret);
      if (edit.digits !== value) onChange?.(edit.digits);
    }

    return (
      <Input
        {...rest}
        ref={ref}
        type="text"
        inputMode="numeric"
        autoComplete="off"
        value={display}
        onChange={(event) => {
          const input = event.currentTarget;
          apply(
            input,
            editTaxId(input.value, input.selectionStart ?? input.value.length, digits, kind),
          );
        }}
        onKeyDown={(event) => {
          const input = event.currentTarget;
          if (input.selectionStart === null || input.selectionStart !== input.selectionEnd) return;
          const edit = deleteAcrossSeparator(display, input.selectionStart, event.key, kind);
          if (!edit) return;
          event.preventDefault();
          apply(input, edit);
        }}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const start = input.selectionStart ?? input.value.length;
          const end = input.selectionEnd ?? input.value.length;
          // A paste fills the room the field has left; a typed digit past it is refused instead.
          const selected = taxIdDigits(input.value.slice(start, end)).length;
          const room = Math.max(0, TAX_ID_MAX_DIGITS[kind] - digits.length + selected);
          const pasted = taxIdDigits(event.clipboardData.getData('text/plain')).slice(0, room);
          const raw = input.value.slice(0, start) + pasted + input.value.slice(end);
          apply(input, editTaxId(raw, start + pasted.length, digits, kind));
        }}
      />
    );
  },
);

TaxIdInput.displayName = 'TaxIdInput';

export { TaxIdInput };
