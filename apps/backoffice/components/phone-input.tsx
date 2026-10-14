'use client';

import { useMemo, useRef, useState, type ChangeEvent, type FocusEvent } from 'react';
import {
  getCountries,
  getCountryCallingCode,
  parsePhoneNumberFromString,
  type CountryCode,
} from 'libphonenumber-js/min';

import { Combobox, InlineLink, Input } from '@repo/ui/components';
import { AR_MOBILE_PREFIX, DEFAULT_PHONE_COUNTRY } from '@/config/phone';
import { useCaretPlacement } from '@/hooks/use-caret-placement';
import { editKind, useInputHistory, type InputSnapshot } from '@/hooks/use-input-history';
import { useFormatters } from '@/lib/i18n/formatters';
import { keepAllowed } from '@/lib/utils/keep-allowed';

// The country list needs room for "Islas Georgias del Sur y Sandwich del Sur (+500)".
const COUNTRY_LIST_MIN_WIDTH = 288;

interface PhoneInputProps {
  id?: string;
  // E.164 (`+5491155550101`) as far as one can be read, '' otherwise; callers check validity.
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  countryLabel: string;
  countrySearchPlaceholder: string;
  countryEmptyLabel: string;
  disabled?: boolean;
  autoFocus?: boolean;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
  /* Offered under the field when an Argentine number reads as a landline but would be a mobile. */
  landlineNotice: { message: string; action: (mobile: string) => string };
}

// A flag is two regional-indicator letters; a platform without flag glyphs shows the letters.
function flagOf(country: CountryCode) {
  return String.fromCodePoint(...[...country].map((letter) => 0x1f1a5 + letter.charCodeAt(0)));
}

function readValue(value: string): { country: CountryCode; national: string } {
  const parsed = value ? parsePhoneNumberFromString(value) : undefined;
  if (parsed?.country) return { country: parsed.country, national: parsed.nationalNumber };
  return { country: DEFAULT_PHONE_COUNTRY, national: value };
}

/*
 * The mobile an Argentine landline-shaped number would be with the 9. Without a 9 or a 15 the digits
 * are a valid landline, and WhatsApp Business can live on one, so this is offered, never applied.
 */
function mobileAlternative(country: CountryCode, national: string) {
  if (country !== 'AR') return null;
  const parsed = parsePhoneNumberFromString(national, country);
  if (!parsed?.isValid() || parsed.nationalNumber.startsWith(AR_MOBILE_PREFIX)) return null;
  const mobile = parsePhoneNumberFromString(
    `+${parsed.countryCallingCode}${AR_MOBILE_PREFIX}${parsed.nationalNumber}`,
  );
  if (!mobile?.isValid()) return null;
  return { international: mobile.formatInternational(), national: mobile.nationalNumber };
}

// What a phone is typed with: digits and the usual separators. The number settles to digits on blur.
const PHONE_CHARACTER = /^[\d\s+\-().]$/;

/*
 * A country picker beside the national number, emitting E.164. A pasted international number
 * ("+598 99 123 456") moves the picker to its country, so the seller never has to split it by hand.
 * The number keeps its own undo history, since a filtered or settled value is one the browser did
 * not write.
 */
export function PhoneInput({
  id,
  value,
  onChange,
  placeholder,
  countryLabel,
  countrySearchPlaceholder,
  countryEmptyLabel,
  disabled = false,
  autoFocus,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
  landlineNotice,
}: PhoneInputProps) {
  const fmt = useFormatters();
  const placeCaret = useCaretPlacement();
  const [field, setField] = useState(() => readValue(value));
  const emitted = useRef(value);
  const nationalRef = useRef<HTMLInputElement>(null);
  // The country rides on each undo step, since a pasted international number changes it.
  const history = useInputHistory({
    ref: nationalRef,
    value: field.national,
    context: () => field.country,
    restore: (snapshot, input) => show(input, snapshot),
  });

  // A value the caller set (a prefill, a reset) is read back in; one this field emitted is not.
  if (value !== emitted.current) {
    emitted.current = value;
    setField(readValue(value));
  }

  const options = useMemo(
    () =>
      getCountries()
        .map((country) => ({
          value: country,
          label: `${fmt.region(country)} (+${getCountryCallingCode(country)})`,
          icon: <span aria-hidden="true">{flagOf(country)}</span>,
        }))
        .sort((a, b) => a.label.localeCompare(b.label, fmt.locale)),
    [fmt],
  );

  const mobile = mobileAlternative(field.country, field.national);

  function emit(next: { country: CountryCode; national: string }) {
    setField(next);
    const parsed = parsePhoneNumberFromString(next.national, next.country);
    const e164 = parsed?.number ?? '';
    emitted.current = e164;
    onChange(e164);
  }

  function show(input: HTMLInputElement, to: InputSnapshot) {
    placeCaret(input, to.start, to.end);
    emit({ country: (to.context as CountryCode | undefined) ?? field.country, national: to.value });
  }

  function handleNationalChange(event: ChangeEvent<HTMLInputElement>) {
    if (history.replay(event)) return;
    const input = event.currentTarget;
    const kept = keepAllowed(
      input.value,
      input.selectionStart ?? input.value.length,
      PHONE_CHARACTER,
    );
    const international = kept.value.trim().startsWith('+')
      ? parsePhoneNumberFromString(kept.value)
      : undefined;
    const to = international?.country
      ? {
          value: international.nationalNumber,
          start: international.nationalNumber.length,
          end: international.nationalNumber.length,
          context: international.country,
        }
      : { value: kept.value, start: kept.caret, end: kept.caret, context: field.country };
    history.record(editKind(event.nativeEvent), history.before(), to);
    show(input, to);
  }

  // A country picked by hand is a step of its own, so undoing the number never reverts it unasked.
  function pickCountry(country: CountryCode) {
    const input = nationalRef.current;
    if (input) {
      const from = history.snapshot(input);
      history.record('other', from, { ...from, context: country });
    }
    emit({ country, national: field.national });
  }

  // A valid number settles to its bare digits, so a typed, pasted or prefilled one reads the same.
  function settleNational(event: FocusEvent<HTMLInputElement>) {
    const parsed = parsePhoneNumberFromString(field.national, field.country);
    if (!parsed?.isValid() || parsed.nationalNumber === field.national) return;
    const national = parsed.nationalNumber;
    const end = national.length;
    history.record('other', history.snapshot(event.currentTarget), {
      value: national,
      start: end,
      end,
      context: field.country,
    });
    setField({ country: field.country, national });
  }

  return (
    <div className="flex flex-col gap-y-2">
      <div className="flex gap-x-2">
        <Combobox
          options={options}
          value={field.country}
          onValueChange={(country) => pickCountry(country as CountryCode)}
          placeholder={countryLabel}
          searchable
          searchPlaceholder={countrySearchPlaceholder}
          emptyLabel={countryEmptyLabel}
          // The flag rides as the trigger's icon; the code gets room for "+" and three digits, the
          // longest there is, so the trigger keeps one width whatever the country.
          triggerLabel={(option) => (
            <span className="inline-block w-[4.5ch] tabular-nums">
              +{getCountryCallingCode(option.value as CountryCode)}
            </span>
          )}
          contentMinWidth={COUNTRY_LIST_MIN_WIDTH}
          disabled={disabled}
          aria-label={countryLabel}
          className="w-auto shrink-0 px-2.5 gap-x-1.5"
        />
        <Input
          ref={history.inputRef}
          id={id}
          type="tel"
          inputMode="tel"
          autoComplete="tel-national"
          value={field.national}
          onChange={handleNationalChange}
          onKeyDown={history.onKeyDown}
          onBlur={settleNational}
          placeholder={placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          aria-invalid={ariaInvalid}
          aria-describedby={ariaDescribedBy}
          containerClassName="min-w-0 flex-1"
        />
      </div>
      <p role="status" className="text-paragraph-xs text-foreground-muted empty:hidden">
        {mobile ? (
          <>
            {landlineNotice.message}{' '}
            <InlineLink asChild>
              <button
                type="button"
                disabled={disabled}
                onClick={() => emit({ country: field.country, national: mobile.national })}
              >
                {landlineNotice.action(mobile.international)}
              </button>
            </InlineLink>
          </>
        ) : null}
      </p>
    </div>
  );
}
