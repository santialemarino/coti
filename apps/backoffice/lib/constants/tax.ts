// An Argentine CUIT is eleven digits; the API stores the raw string the seller typed.
export const TAX_ID_LENGTH = 11;

export const TAX_ID_PATTERN = /^\d{11}$/;

// Legacy rows may carry hyphens; the field only ever shows the eleven digits.
export function taxIdDigits(raw: string | null | undefined): string {
  return raw?.replace(/\D/g, '') ?? '';
}
