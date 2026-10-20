/*
 * Drops every character `allowed` (a one-character, non-global pattern) refuses and keeps the caret
 * after the characters it followed — for a field that filters what is typed without formatting it.
 */
export function keepAllowed(
  raw: string,
  caret: number,
  allowed: RegExp,
): { value: string; caret: number } {
  const keeps = (char: string) => allowed.test(char);
  return {
    value: [...raw].filter(keeps).join(''),
    caret: [...raw.slice(0, caret)].filter(keeps).length,
  };
}
