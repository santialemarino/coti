import type { ApiErrorCode } from '@/lib/api/errors';

/* What both next-intl entry points hand back, narrowed to what resolving a code needs. */
export interface CatalogReader {
  (key: string): string;
  has(key: string): boolean;
}

/*
 * The one place an API refusal becomes a sentence. Every code has an entry under `errors`, and a
 * flow overrides the ones it words differently by repeating the code under its own `errors` — so
 * a screen never carries a ladder of `code === …`, and three flows never carry three spellings of
 * "no tenés permiso".
 *
 * The namespace is walked back a segment at a time, most specific first: `users.passwordReset`
 * tries `users.passwordReset.errors.X`, then `users.errors.X`, then `errors.X`. That is what lets
 * one action word a code the rest of its flow shares — a 422 means "that user is deactivated"
 * when mailing a link and "check these values" everywhere else.
 */
export function apiErrorMessage(
  t: CatalogReader,
  namespace: string | undefined,
  code: ApiErrorCode = 'INTERNAL',
): string {
  const segments = namespace ? namespace.split('.') : [];
  while (segments.length > 0) {
    const candidate = `${segments.join('.')}.errors.${code}`;
    if (t.has(candidate)) return t(candidate);
    segments.pop();
  }
  return t(`errors.${code}`);
}
