import { describe, expect, it } from 'vitest';

import {
  API_ERROR_CODES,
  ApiError,
  codeForStatus,
  errorCodeOf,
  knownErrorCode,
} from '@/lib/api/errors';
import messages from '@/translations/es.json';

describe('errorCodeOf', () => {
  it('reads the code off an ApiError', () => {
    expect(errorCodeOf(new ApiError('EMAIL_TAKEN', 409))).toBe('EMAIL_TAKEN');
  });

  it.each([new Error('boom'), 'a string', null, undefined])(
    'calls anything else INTERNAL (%p)',
    (thrown) => {
      expect(errorCodeOf(thrown)).toBe('INTERNAL');
    },
  );
});

describe('knownErrorCode', () => {
  it('accepts every code the catalog can word', () => {
    API_ERROR_CODES.forEach((code) => expect(knownErrorCode(code)).toBe(code));
  });

  /*
   * A code the API adds before this app knows it must not reach the catalog, which has no wording
   * for it — next-intl would render the key. Refusing it here is what makes the status fallback
   * cover the gap instead.
   */
  it.each(['QUOTE_FROZEN', 'email_taken', '', null, undefined, 42])(
    'refuses anything it cannot word (%p)',
    (value) => {
      expect(knownErrorCode(value)).toBeUndefined();
    },
  );
});

describe('codeForStatus', () => {
  it('never answers with a code the client mints for itself', () => {
    const statuses = [200, 400, 401, 403, 404, 409, 413, 418, 422, 429, 500, 503];
    const derived = statuses.map(codeForStatus);
    expect(derived).not.toContain('UNREACHABLE');
    expect(derived).not.toContain('SESSION_EXPIRED');
  });
});

/*
 * The resolver falls back to `errors.<code>` for anything a flow does not override, so a code with
 * no entry there renders its own key. Nothing else catches that: the wire types agree and the
 * screen still compiles.
 */
describe('the shared catalog', () => {
  it('words every code', () => {
    const missing = API_ERROR_CODES.filter((code) => !(code in messages.errors));
    expect(missing).toEqual([]);
  });

  it('carries no entry for a code that no longer exists', () => {
    const stale = Object.keys(messages.errors).filter(
      (key) => !API_ERROR_CODES.some((code) => code === key),
    );
    expect(stale).toEqual([]);
  });
});
