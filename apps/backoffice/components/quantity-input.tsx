'use client';

import { forwardRef } from 'react';

import { Input } from '@repo/ui/components';
import {
  blockAllSeparators,
  blockScientificKeys,
  blockSignKeys,
  composeKeyHandlers,
  sanitizeIntegerPaste,
  stepCanonical,
} from '@/lib/i18n/numeric-input';

interface QuantityInputProps {
  /* A digit-only string. Range checks stay in the schema; this only guarantees the shape. */
  value?: string;
  onChange?: (value: string) => void;
  onBlur?: () => void;
  onFocus?: React.FocusEventHandler<HTMLInputElement>;
  name?: string;
  placeholder?: string;
  disabled?: boolean;
  min?: number;
  step?: number;
  /* Runs after the field's own rules, and only when they let the key through. */
  onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
  suffix?: React.ReactNode;
  className?: string;
  containerClassName?: string;
  'aria-label'?: string;
  'aria-invalid'?: boolean | 'true' | 'false';
  'aria-describedby'?: string;
  id?: string;
}

/*
 * The whole-number field: how many bags, how many days a quote stays valid, how many users. Digits
 * only — no separators, no sign, no exponent — and, like `AmountInput`, `type="text"` so the mouse
 * wheel cannot rewrite it while the page scrolls past. The arrow keys step it.
 *
 * The rule across the app: money and anything with a fraction is `AmountInput`; a count is this.
 * Neither is `<input type="number">`.
 */
const QuantityInput = forwardRef<HTMLInputElement, QuantityInputProps>(
  ({ value = '', onChange, onBlur, onKeyDown, name, min = 0, step = 1, ...rest }, ref) => {
    const runKeyRules = composeKeyHandlers(blockSignKeys, blockScientificKeys, blockAllSeparators);

    return (
      <Input
        {...rest}
        ref={ref}
        type="text"
        inputMode="numeric"
        name={name}
        value={value}
        /* Strips anything a keystroke rule never sees: IME composition, autofill, dropped text. */
        onChange={(event) => onChange?.(event.target.value.replace(/[^0-9]/g, ''))}
        onBlur={onBlur}
        onKeyDown={(event) => {
          runKeyRules(event);
          if (event.defaultPrevented) return;
          const stepped = stepCanonical(event.key, value, { step, min, maxDecimals: 0 });
          if (stepped !== null) {
            event.preventDefault();
            onChange?.(stepped);
            return;
          }
          onKeyDown?.(event);
        }}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const start = input.selectionStart ?? input.value.length;
          const end = input.selectionEnd ?? input.value.length;
          const pasted = sanitizeIntegerPaste(event.clipboardData.getData('text/plain'));
          onChange?.(input.value.slice(0, start) + pasted + input.value.slice(end));
        }}
        autoComplete="off"
      />
    );
  },
);

QuantityInput.displayName = 'QuantityInput';

export { QuantityInput };
