/*
 * The password policy, mirrored from the API so a form can refuse before the round trip. The API
 * stays the authority — it applies the same rules on every path that stores a password and answers
 * 422 when this mirror drifts.
 */

// Mirrors AUTH_PASSWORD_MIN_LENGTH's default.
export const PASSWORD_MIN_LENGTH = 12;

// bcrypt hashes the first 72 **bytes** and refuses more, so the cap is counted in bytes: an accented
// password reaches it in fewer characters than an ASCII one.
export const PASSWORD_MAX_BYTES = 72;

// A coarse guard on the input itself, in characters, so the field stops somewhere sane while the
// byte count above is what actually decides.
export const PASSWORD_MAX_LENGTH = 72;

// The cap on a password being presented rather than chosen, which is what the API accepts on the
// routes that only compare one. Wider than the cap above, so an older password still logs in.
export const SECRET_MAX_LENGTH = 128;

const UPPERCASE = /\p{Lu}/u;
const LOWERCASE = /\p{Ll}/u;
const NUMBER = /\p{Nd}/u;
// Anything that is not a letter or a number, so an ordinary hyphen or period counts.
const SYMBOL = /[^\p{L}\p{N}]/u;

/* In the order the meter lists them, which is the order they are easiest to fix in. */
export const PASSWORD_CHECKS = ['length', 'uppercase', 'lowercase', 'number', 'symbol'] as const;

export type PasswordCheck = (typeof PASSWORD_CHECKS)[number];

export function passwordByteLength(password: string): number {
  return new TextEncoder().encode(password).length;
}

export function passwordChecks(password: string): Record<PasswordCheck, boolean> {
  return {
    length: [...password].length >= PASSWORD_MIN_LENGTH,
    uppercase: UPPERCASE.test(password),
    lowercase: LOWERCASE.test(password),
    number: NUMBER.test(password),
    symbol: SYMBOL.test(password),
  };
}

/* Length is checked on its own, because it has its own message; this is the rest of the policy. */
export function hasEveryCharacterClass(password: string): boolean {
  const checks = passwordChecks(password);
  return checks.uppercase && checks.lowercase && checks.number && checks.symbol;
}
