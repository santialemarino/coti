'use client';

import { useMemo, useRef, useState } from 'react';
import {
  getCountries,
  getCountryCallingCode,
  parsePhoneNumberFromString,
  type CountryCode,
} from 'libphonenumber-js/min';

import { Combobox, InlineLink, Input } from '@repo/ui/components';
import { AR_MOBILE_PREFIX, DEFAULT_PHONE_COUNTRY } from '@/config/phone';
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
const PHONE_CHARACTERS = /[^\d\s+\-().]/g;

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
  landlineNotice,
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

  const mobile = mobileAlternative(field.country, field.national);

  function emit(next: { country: CountryCode; national: string }) {
    setField(next);
    const parsed = parsePhoneNumberFromString(next.national, next.country);
    const e164 = parsed?.number ?? '';
    emitted.current = e164;
    onChange(e164);
  }

  function handleNationalChange(typed: string) {
    const raw = typed.replace(PHONE_CHARACTERS, '');
    const international = raw.trim().startsWith('+') ? parsePhoneNumberFromString(raw) : undefined;
    if (international?.country) {
      emit({ country: international.country, national: international.nationalNumber });
      return;
    }
    emit({ country: field.country, national: raw });
  }

  // A valid number settles to its bare digits, so a typed, pasted or prefilled one reads the same.
  function settleNational() {
    const parsed = parsePhoneNumberFromString(field.national, field.country);
    if (parsed?.isValid() && parsed.nationalNumber !== field.national) {
      setField({ country: field.country, national: parsed.nationalNumber });
    }
  }

  return (
    <div className="flex flex-col gap-y-2">
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
          id={id}
          type="tel"
          inputMode="tel"
          autoComplete="tel-national"
          value={field.national}
          onChange={(event) => handleNationalChange(event.target.value)}
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
