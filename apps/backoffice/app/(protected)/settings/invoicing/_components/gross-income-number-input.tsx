'use client';

import { forwardRef } from 'react';

import { Input } from '@repo/ui/components';
import { GROSS_INCOME_CHARACTER } from '@/app/(protected)/settings/invoicing/form-schema';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import { keepAllowed } from '@/lib/utils/tax-id';

type GrossIncomeNumberInputProps = Omit<React.ComponentProps<typeof Input>, 'onChange'> & {
  onChange: (value: string) => void;
};

/*
 * An Ingresos Brutos number keeps its digits and hyphens as typed: provinces write it in different
 * shapes, so nothing is reformatted, only filtered. The caret lives here so a keystroke renders
 * this field, not the whole form.
 */
const GrossIncomeNumberInput = forwardRef<HTMLInputElement, GrossIncomeNumberInputProps>(
  ({ onChange, ...props }, ref) => {
    const placeCaret = useCaretPlacement();

    return (
      <Input
        ref={ref}
        {...props}
        onChange={(event) => {
          const input = event.currentTarget;
          const caret = input.selectionStart ?? input.value.length;
          const kept = keepAllowed(input.value, caret, GROSS_INCOME_CHARACTER);
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
