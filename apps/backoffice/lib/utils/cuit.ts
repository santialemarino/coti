const CUIT_DIGITS = 11;
const CUIT_WEIGHTS = [5, 4, 3, 2, 7, 6, 5, 4, 3, 2];

// The CUIT check digit, as the API and ARCA compute it: modulo 11 over the first ten digits. A value
// that is not eleven digits long is not a CUIT, so it has no check digit to fail.
export function hasValidCuitCheckDigit(raw: string): boolean {
  const digits = raw.replace(/\D/g, '');
  if (digits.length !== CUIT_DIGITS) return true;
  const sum = CUIT_WEIGHTS.reduce((total, weight, i) => total + Number(digits[i]) * weight, 0);
  const remainder = 11 - (sum % 11);
  const check = remainder === 11 ? 0 : remainder === 10 ? 9 : remainder;
  return Number(digits[10]) === check;
}

// Whether a value is a whole CUIT ARCA accepts: eleven digits, separators aside, and its check digit.
export function isValidCuit(raw: string): boolean {
  return raw.replace(/\D/g, '').length === CUIT_DIGITS && hasValidCuitCheckDigit(raw);
}
