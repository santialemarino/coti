// The Argentine convention the product writes money in: "132.467,89".
const GROUP_SEPARATOR = '.';
const DECIMAL_SEPARATOR = ',';
const MAX_DECIMALS = 2;
// NUMERIC(14,2) leaves twelve digits before the comma, so a longer entry could not be stored.
const MAX_INTEGER_DIGITS = 12;

/*
 * Groups an amount as it is typed, the way a phone's calculator does: the seller enters digits
 * and a comma, and the thousand separators appear and move on their own. Anything else is
 * dropped rather than rejected, so a pasted "$ 132.467,89" reads as the number it shows.
 */
export function maskMoneyInput(raw: string): string {
  const [integerPart, ...rest] = raw.split(DECIMAL_SEPARATOR);
  const digits = onlyDigits(integerPart ?? '').slice(0, MAX_INTEGER_DIGITS);
  const grouped = groupDigits(digits.replace(/^0+(?=\d)/, ''));

  // A trailing comma is a decimal part the seller has started and not filled yet, so it has to
  // survive the round trip through this function or it could never be typed.
  if (rest.length === 0) return grouped;
  const decimals = onlyDigits(rest.join('')).slice(0, MAX_DECIMALS);
  return `${grouped || '0'}${DECIMAL_SEPARATOR}${decimals}`;
}

// "132.467,89" → "132467.89", the decimal string the API speaks. Empty for a blank entry.
export function moneyInputToDecimal(masked: string): string {
  const [integerPart, decimalPart] = masked.split(DECIMAL_SEPARATOR);
  const integer = onlyDigits(integerPart ?? '');
  const decimals = onlyDigits(decimalPart ?? '').slice(0, MAX_DECIMALS);
  if (!integer && !decimals) return '';
  return decimals ? `${integer || '0'}.${decimals}` : integer;
}

// "132467.89" → "132.467,89", for seeding the field from a stored amount.
export function decimalToMoneyInput(decimal: string): string {
  const [integer = '', decimals = ''] = decimal.split('.');
  const grouped = groupDigits(onlyDigits(integer));
  const fraction = onlyDigits(decimals).slice(0, MAX_DECIMALS).padEnd(MAX_DECIMALS, '0');
  return `${grouped || '0'}${DECIMAL_SEPARATOR}${fraction}`;
}

function onlyDigits(value: string): string {
  return value.replace(/\D/g, '');
}

function groupDigits(digits: string): string {
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, GROUP_SEPARATOR);
}
