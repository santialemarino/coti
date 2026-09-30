import type { CountryCode } from 'libphonenumber-js/min';

// The country a phone field starts on: Coti's corralones and their clients are in Argentina.
export const DEFAULT_PHONE_COUNTRY: CountryCode = 'AR';

// In E.164 an Argentine mobile carries a 9 between the country and the area code, where a landline has none.
export const AR_MOBILE_PREFIX = '9';
