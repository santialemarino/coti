'use client';

import { forwardRef } from 'react';

import { Input } from '@repo/ui/components';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import {
  editKind,
  useInputHistory,
  type EditKind,
  type InputSnapshot,
} from '@/hooks/use-input-history';
import {
  blockAllSeparators,
  blockScientificKeys,
  blockSignKeys,
  composeKeyHandlers,
  sanitizeIntegerPaste,
  stepCanonical,
} from '@/lib/i18n/numeric-input';
import { keepAllowed } from '@/lib/utils/keep-allowed';

const DIGIT = /^\d$/;

interface QuantityInputProps {
  /* A digit-only string. Range checks stay in the schema; this only guarantees the shape. */
  value?: string;
  onChange?: (value: string) => void;
  onBlur?: () => void;
  onFocus?: React.FocusEventHandler<HTMLInputElement>;
  name?: string;
  placeholder?: string;
  disabled?: boolean;
  maxLength?: number;
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
 * Neither is `<input type="number">`. Filtering is a write the browser does not record, so the field
 * keeps its own undo history.
 */
const QuantityInput = forwardRef<HTMLInputElement, QuantityInputProps>(
  ({ value = '', onChange, onBlur, onKeyDown, name, min = 0, step = 1, ...rest }, ref) => {
    const placeCaret = useCaretPlacement();
    const history = useInputHistory({
      ref,
      value,
      restore: (snapshot, input) => show(input, snapshot),
    });
    const runKeyRules = composeKeyHandlers(blockSignKeys, blockScientificKeys, blockAllSeparators);

    function show(input: HTMLInputElement, to: InputSnapshot) {
      placeCaret(input, to.start, to.end);
      if (to.value !== value) onChange?.(to.value);
    }

    function apply(
      input: HTMLInputElement,
      edited: EditKind,
      to: InputSnapshot,
      from: InputSnapshot,
    ) {
      history.record(edited, from, to);
      show(input, to);
    }

    return (
      <Input
        {...rest}
        ref={history.inputRef}
        type="text"
        inputMode="numeric"
        name={name}
        value={value}
        /* Strips anything a keystroke rule never sees: IME composition, autofill, dropped text. */
        onChange={(event) => {
          if (history.replay(event)) return;
          const input = event.currentTarget;
          const kept = keepAllowed(input.value, input.selectionStart ?? input.value.length, DIGIT);
          const to = { value: kept.value, start: kept.caret, end: kept.caret };
          apply(input, editKind(event.nativeEvent), to, history.before());
        }}
        onBlur={onBlur}
        onKeyDown={(event) => {
          if (history.onKeyDown(event)) return;
          runKeyRules(event);
          if (event.defaultPrevented) return;
          const stepped = stepCanonical(event.key, value, { step, min, maxDecimals: 0 });
          if (stepped !== null) {
            event.preventDefault();
            const input = event.currentTarget;
            const to = { value: stepped, start: stepped.length, end: stepped.length };
            apply(input, 'other', to, history.snapshot(input));
            return;
          }
          onKeyDown?.(event);
        }}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const from = history.snapshot(input);
          // A paste fills the room `maxLength` leaves, as typing into the field would.
          const room = (rest.maxLength ?? Infinity) - value.length + (from.end - from.start);
          const pasted = sanitizeIntegerPaste(event.clipboardData.getData('text/plain')).slice(
            0,
            Math.max(0, room),
          );
          const caret = from.start + pasted.length;
          const raw = value.slice(0, from.start) + pasted + value.slice(from.end);
          apply(input, 'other', { value: raw, start: caret, end: caret }, from);
        }}
        autoComplete="off"
      />
    );
  },
);

QuantityInput.displayName = 'QuantityInput';

export { QuantityInput };
