'use client';

import { forwardRef, useEffect, useState } from 'react';
import { useLocale } from 'next-intl';

import { Input } from '@repo/ui/components';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import {
  editKind,
  useInputHistory,
  type EditKind,
  type InputSnapshot,
} from '@/hooks/use-input-history';
import { CURRENCY_DECIMALS } from '@/lib/i18n/currency';
import {
  blockDecimalWhenIntegral,
  blockScientificKeys,
  blockSignKeys,
  composeKeyHandlers,
  editAmount,
  formatAmountForInput,
  getGroupSeparator,
  limitDecimalsInString,
  normalizeAmountFromInput,
  sanitizeDecimalPaste,
  stepCanonical,
  type AmountEdit,
} from '@/lib/i18n/numeric-input';

interface AmountInputProps {
  /* Canonical `.`-decimal string — exactly what the API sends and receives. */
  value?: string;
  onChange?: (value: string) => void;
  onBlur?: () => void;
  name?: string;
  placeholder?: string;
  disabled?: boolean;
  /* ISO 4217 code; its sub-unit precision caps the fraction. Ignored when `maxDecimals` is given. */
  currency?: string;
  /* Explicit fraction cap. `0` forbids decimals — reach for `QuantityInput` instead when counting. */
  maxDecimals?: number;
  /* How much one arrow press moves the value. */
  step?: number;
  /* Runs after the field's own rules, and only when they let the key through. */
  onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  className?: string;
  containerClassName?: string;
  'aria-label'?: string;
  'aria-invalid'?: boolean | 'true' | 'false';
  'aria-describedby'?: string;
  id?: string;
}

/*
 * The money field. Holds a canonical `.`-decimal string in form state and shows the locale's
 * notation as it is typed — "1.234.567,89" in es-AR — so an Argentine seller typing `1234,56` is
 * not silently rejected the way `<input type="number">` rejects it.
 *
 * It is `type="text"` on purpose. A number input binds the mouse wheel, so scrolling a long form
 * past a focused price rewrites it without a keystroke; the arrows people actually want are handled
 * here instead, over the canonical value.
 *
 * Every edit runs one pipeline, `editAmount`: sanitise the raw string → canonicalise → truncate to
 * the currency's precision → regroup for display → replace the caret by counting digits rather than
 * characters, because the group separators are what moved. Each one is a step of the field's own
 * undo history, since the browser keeps none for a value it did not write.
 */
const AmountInput = forwardRef<HTMLInputElement, AmountInputProps>(
  (
    { value = '', onChange, onBlur, onKeyDown, name, currency, maxDecimals, step = 1, ...rest },
    ref,
  ) => {
    const locale = useLocale();
    const group = getGroupSeparator(locale);
    const cap = maxDecimals ?? (currency ? (CURRENCY_DECIMALS[currency] ?? 2) : undefined);
    const placeCaret = useCaretPlacement();
    const [display, setDisplay] = useState(() => formatAmountForInput(value, locale));
    const history = useInputHistory({
      ref,
      value: display,
      restore: (snapshot, input) => show(input, snapshot),
    });

    /*
     * Re-sync when the canonical value changes from outside — a form reset, an edited row. Deps are
     * the value only: `display` is read to decide whether a re-sync is needed, and depending on it
     * would re-run this on every keystroke.
     */
    useEffect(() => {
      const capped = limitDecimalsInString(value, '.', cap);
      if (normalizeAmountFromInput(display, locale) !== capped) {
        setDisplay(formatAmountForInput(capped, locale));
      }
      if (capped !== value) onChange?.(capped);
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [value, locale]);

    function show(input: HTMLInputElement, to: InputSnapshot) {
      const canonical = normalizeAmountFromInput(to.value, locale);
      setDisplay(to.value);
      placeCaret(input, to.start, to.end);
      if (canonical !== value) onChange?.(canonical);
    }

    function apply(
      input: HTMLInputElement,
      edited: EditKind,
      edit: AmountEdit,
      from: InputSnapshot,
    ) {
      const to = { value: edit.display, start: edit.caret, end: edit.caret };
      history.record(edited, from, to);
      show(input, to);
    }

    // A group separator typed at the end is shown only while typing; leaving the field drops it.
    function settle(input: HTMLInputElement) {
      const settled = formatAmountForInput(normalizeAmountFromInput(display, locale), locale);
      if (settled === display) return;
      const end = settled.length;
      history.record('other', history.snapshot(input), { value: settled, start: end, end });
      setDisplay(settled);
    }

    const runKeyRules = composeKeyHandlers(
      blockSignKeys,
      blockScientificKeys,
      blockDecimalWhenIntegral(locale, cap),
    );

    function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
      if (history.onKeyDown(event)) return;
      runKeyRules(event);
      if (event.defaultPrevented) return;
      const input = event.currentTarget;

      const stepped = stepCanonical(event.key, normalizeAmountFromInput(display, locale), {
        step,
        maxDecimals: cap,
      });
      if (stepped !== null) {
        event.preventDefault();
        const next = formatAmountForInput(stepped, locale);
        const edit = { canonical: stepped, display: next, caret: next.length };
        apply(input, 'other', edit, history.snapshot(input));
        return;
      }

      /*
       * Backspace or Delete onto a group separator takes the adjacent digit. Removing the separator
       * alone regroups it straight back, which traps the caret against it forever.
       */
      const start = input.selectionStart;
      const modified = event.altKey || event.metaKey || event.ctrlKey;
      if (start !== null && start === input.selectionEnd && !modified) {
        const from = history.snapshot(input);
        if (event.key === 'Backspace' && start > 1 && display[start - 1] === group) {
          event.preventDefault();
          const raw = display.slice(0, start - 2) + display.slice(start - 1);
          apply(input, 'delete', editAmount(raw, start - 2, display, locale, cap), from);
        } else if (event.key === 'Delete' && display[start] === group) {
          event.preventDefault();
          const raw = display.slice(0, start) + display.slice(start + 2);
          apply(input, 'delete', editAmount(raw, start, display, locale, cap), from);
        }
      }
      if (!event.defaultPrevented) onKeyDown?.(event);
    }

    return (
      <Input
        {...rest}
        ref={history.inputRef}
        type="text"
        inputMode="decimal"
        name={name}
        value={display}
        onChange={(event) => {
          if (history.replay(event)) return;
          const input = event.currentTarget;
          const caret = input.selectionStart ?? input.value.length;
          const edit = editAmount(input.value, caret, display, locale, cap);
          apply(input, editKind(event.nativeEvent), edit, history.before());
        }}
        onBlur={(event) => {
          settle(event.currentTarget);
          onBlur?.();
        }}
        onKeyDown={handleKeyDown}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const from = history.snapshot(input);
          const pasted = sanitizeDecimalPaste(event.clipboardData.getData('text/plain'), locale);
          const raw = display.slice(0, from.start) + pasted + display.slice(from.end);
          const edit = editAmount(raw, from.start + pasted.length, display, locale, cap);
          apply(input, 'other', edit, from);
        }}
        autoComplete="off"
        className={rest.className}
      />
    );
  },
);

AmountInput.displayName = 'AmountInput';

export { AmountInput };
