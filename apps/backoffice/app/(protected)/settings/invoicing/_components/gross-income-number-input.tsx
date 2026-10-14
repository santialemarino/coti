'use client';

import { forwardRef } from 'react';

import { Input } from '@repo/ui/components';
import { GROSS_INCOME_CHARACTER } from '@/app/(protected)/settings/invoicing/form-schema';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import { editKind, useInputHistory } from '@/hooks/use-input-history';
import { keepAllowed } from '@/lib/utils/tax-id';

type GrossIncomeNumberInputProps = Omit<React.ComponentProps<typeof Input>, 'onChange'> & {
  value?: string;
  onChange: (value: string) => void;
};

/*
 * An Ingresos Brutos number keeps its digits and hyphens as typed: provinces write it in different
 * shapes, so nothing is reformatted, only filtered. The caret lives here so a keystroke renders
 * this field, not the whole form, and so does the undo history a filtered write would cost it.
 */
const GrossIncomeNumberInput = forwardRef<HTMLInputElement, GrossIncomeNumberInputProps>(
  ({ value = '', onChange, onKeyDown, ...props }, ref) => {
    const placeCaret = useCaretPlacement();
    const history = useInputHistory({
      ref,
      value,
      restore: (snapshot, input) => {
        placeCaret(input, snapshot.start, snapshot.end);
        onChange(snapshot.value);
      },
    });

    return (
      <Input
        {...props}
        ref={history.inputRef}
        value={value}
        onKeyDown={(event) => {
          onKeyDown?.(event);
          history.onKeyDown(event);
        }}
        onChange={(event) => {
          const input = event.currentTarget;
          const caret = input.selectionStart ?? input.value.length;
          const kept = keepAllowed(input.value, caret, GROSS_INCOME_CHARACTER);
          history.record(editKind(event.nativeEvent), history.before(), {
            value: kept.value,
            start: kept.caret,
            end: kept.caret,
          });
          placeCaret(input, kept.caret);
          onChange(kept.value);
        }}
        inputMode="numeric"
        autoComplete="off"
      />
    );
  },
);
GrossIncomeNumberInput.displayName = 'GrossIncomeNumberInput';

export { GrossIncomeNumberInput };
