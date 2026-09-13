'use client';

import { forwardRef, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useLocale } from 'next-intl';

import { Input } from '@repo/ui/components';
import { CURRENCY_DECIMALS } from '@/lib/i18n/currency';
import {
  blockDecimalWhenIntegral,
  blockScientificKeys,
  blockSecondDecimal,
  blockSignKeys,
  blockWrongLocaleDecimal,
  composeKeyHandlers,
  formatAmountForInput,
  getDecimalSeparator,
  getGroupSeparator,
  limitDecimalsInString,
  mapCaretAfterRegroup,
  normalizeAmountFromInput,
  sanitizeDecimalChars,
  sanitizeDecimalPaste,
  stepCanonical,
} from '@/lib/i18n/numeric-input';

/* The caret has to be written before paint; on the server there is no paint and no warning to earn. */
const useIsomorphicLayoutEffect = typeof document !== 'undefined' ? useLayoutEffect : useEffect;

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
 * Every mutation runs one pipeline: sanitise the raw string → canonicalise → truncate to the
 * currency's precision → regroup for display → replace the caret by counting digits rather than
 * characters, because the group separators are what moved.
 */
const AmountInput = forwardRef<HTMLInputElement, AmountInputProps>(
  ({ value = '', onChange, onBlur, name, currency, maxDecimals, step = 1, ...rest }, ref) => {
    const locale = useLocale();
    const decimal = getDecimalSeparator(locale);
    const group = getGroupSeparator(locale);
    const cap = maxDecimals ?? (currency ? (CURRENCY_DECIMALS[currency] ?? 2) : undefined);

    const [display, setDisplay] = useState(() => formatAmountForInput(value, locale));
    const innerRef = useRef<HTMLInputElement | null>(null);
    /* Caret to restore once the regrouped value has committed; null means nothing pending. */
    const pendingCaret = useRef<number | null>(null);

    function setRefs(node: HTMLInputElement | null) {
      innerRef.current = node;
      if (typeof ref === 'function') ref(node);
      else if (ref) ref.current = node;
    }

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

    useIsomorphicLayoutEffect(() => {
      const caret = pendingCaret.current;
      if (caret === null || innerRef.current === null) return;
      pendingCaret.current = null;
      innerRef.current.setSelectionRange(caret, caret);
    });

    function applyRaw(rawDisplay: string, rawCaret: number) {
      const canonical = limitDecimalsInString(
        normalizeAmountFromInput(sanitizeDecimalChars(rawDisplay, locale), locale),
        '.',
        cap,
      );
      const next = formatAmountForInput(canonical, locale);
      /* Queue the caret only when the display really changes, so the ref is never left dangling. */
      if (next !== display) {
        pendingCaret.current = mapCaretAfterRegroup(rawDisplay, rawCaret, next, decimal);
        setDisplay(next);
      }
      onChange?.(canonical);
    }

    const runKeyRules = composeKeyHandlers(
      blockSignKeys,
      blockScientificKeys,
      blockWrongLocaleDecimal(locale),
      blockSecondDecimal(locale, display),
      blockDecimalWhenIntegral(locale, cap),
    );

    function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
      runKeyRules(event);
      if (event.defaultPrevented) return;

      const stepped = stepCanonical(event.key, normalizeAmountFromInput(display, locale), {
        step,
        maxDecimals: cap,
      });
      if (stepped !== null) {
        event.preventDefault();
        const next = formatAmountForInput(stepped, locale);
        pendingCaret.current = next.length;
        setDisplay(next);
        onChange?.(stepped);
        return;
      }

      /*
       * Backspace or Delete onto a group separator takes the adjacent digit. Removing the separator
       * alone regroups it straight back, which traps the caret against it forever.
       */
      const input = event.currentTarget;
      const start = input.selectionStart;
      if (start === null || start !== input.selectionEnd) return;
      if (event.key === 'Backspace' && start > 1 && display[start - 1] === group) {
        event.preventDefault();
        applyRaw(display.slice(0, start - 2) + display.slice(start - 1), start - 2);
      } else if (event.key === 'Delete' && display[start] === group) {
        event.preventDefault();
        applyRaw(display.slice(0, start) + display.slice(start + 2), start);
      }
    }

    return (
      <Input
        {...rest}
        ref={setRefs}
        type="text"
        inputMode="decimal"
        name={name}
        value={display}
        onChange={(event) =>
          applyRaw(event.currentTarget.value, event.currentTarget.selectionStart ?? 0)
        }
        onBlur={onBlur}
        onKeyDown={handleKeyDown}
        onPaste={(event) => {
          event.preventDefault();
          const input = event.currentTarget;
          const start = input.selectionStart ?? input.value.length;
          const end = input.selectionEnd ?? input.value.length;
          const pasted = sanitizeDecimalPaste(event.clipboardData.getData('text/plain'), locale);
          applyRaw(
            input.value.slice(0, start) + pasted + input.value.slice(end),
            start + pasted.length,
          );
        }}
        autoComplete="off"
        className={rest.className}
      />
    );
  },
);

AmountInput.displayName = 'AmountInput';

export { AmountInput };
