'use client';

import { useMemo, useRef, useState } from 'react';
import {
  getCountries,
  getCountryCallingCode,
  parsePhoneNumberFromString,
  type CountryCode,
} from 'libphonenumber-js/min';

import { Combobox, Input } from '@repo/ui/components';
import { DEFAULT_PHONE_COUNTRY } from '@/config/phone';
import { useFormatters } from '@/lib/i18n/formatters';

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
 * A country picker beside the national number, emitting E.164. A pasted international number
 * ("+598 99 123 456") moves the picker to its country, so the seller never has to split it by hand.
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
}: PhoneInputProps) {
  const fmt = useFormatters();
  const [field, setField] = useState(() => readValue(value));
  const emitted = useRef(value);

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

  function emit(next: { country: CountryCode; national: string }) {
    setField(next);
    const parsed = parsePhoneNumberFromString(next.national, next.country);
    const e164 = parsed?.number ?? '';
    emitted.current = e164;
    onChange(e164);
  }

  function handleNationalChange(raw: string) {
    const international = raw.trim().startsWith('+') ? parsePhoneNumberFromString(raw) : undefined;
    if (international?.country) {
      emit({ country: international.country, national: international.nationalNumber });
      return;
    }
    emit({ country: field.country, national: raw });
  }

  return (
    <div className="flex gap-x-2">
      <Combobox
        options={options}
        value={field.country}
        onValueChange={(country) =>
          emit({ country: country as CountryCode, national: field.national })
        }
        placeholder={countryLabel}
        searchable
        searchPlaceholder={countrySearchPlaceholder}
        emptyLabel={countryEmptyLabel}
        // The flag rides as the trigger's icon; the code gets room for "+" and three digits, the
        // longest there is, so the trigger keeps one width whatever the country.
        triggerLabel={(option) => (
          <span className="inline-block min-w-[4ch] tabular-nums">
            +{getCountryCallingCode(option.value as CountryCode)}
          </span>
        )}
        contentMinWidth={COUNTRY_LIST_MIN_WIDTH}
        disabled={disabled}
        aria-label={countryLabel}
        className="w-auto shrink-0 px-3"
      />
      <Input
        id={id}
        type="tel"
        inputMode="tel"
        autoComplete="tel-national"
        value={field.national}
        onChange={(event) => handleNationalChange(event.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        autoFocus={autoFocus}
        aria-invalid={ariaInvalid}
        aria-describedby={ariaDescribedBy}
        containerClassName="min-w-0 flex-1"
      />
    </div>
  );
}
