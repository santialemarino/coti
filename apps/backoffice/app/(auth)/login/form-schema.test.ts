import { describe, expect, it } from 'vitest';

import { loginSchema } from '@/app/(auth)/login/form-schema';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

const VALID = { email: 'admin@corralon.test', password: 'coti1234', rememberMe: false };

describe('loginSchema', () => {
  it('accepts a complete set of credentials', () => {
    expect(loginSchema().safeParse(VALID).success).toBe(true);
  });

  it.each(['no-at-sign', 'missing@tld', '@corralon.test', ''])(
    'rejects %p as an address',
    (email) => {
      expect(loginSchema().safeParse({ ...VALID, email }).success).toBe(false);
    },
  );

  // Mirrors the API's own floor so the form can reject before the round trip; the API stays
  // the authority and answers 422 if the two ever drift.
  it('rejects a password below the shared minimum', () => {
    const password = 'a'.repeat(PASSWORD_MIN_LENGTH - 1);
    expect(loginSchema().safeParse({ ...VALID, password }).success).toBe(false);
  });

  it('accepts a password exactly at the minimum', () => {
    const password = 'a'.repeat(PASSWORD_MIN_LENGTH);
    expect(loginSchema().safeParse({ ...VALID, password }).success).toBe(true);
  });

  /*
   * The factory takes the translator so a message is a catalog key rather than Spanish baked
   * into the schema — which is what lets the server action re-validate with the same schema
   * and ignore the wording.
   */
  it('carries the translator’s message through to the issue', () => {
    const schema = loginSchema((key) => `translated:${key}`);
    const result = schema.safeParse({ ...VALID, email: 'nope' });

    expect(result.success).toBe(false);
    expect(result.error?.issues[0]?.message).toBe('translated:email.invalid');
  });

  it('defaults to the raw key when no translator is given', () => {
    const result = loginSchema().safeParse({ ...VALID, email: 'nope' });
    expect(result.error?.issues[0]?.message).toBe('email.invalid');
  });

  it('requires rememberMe to be present, since the session length depends on it', () => {
    const withoutFlag = { email: VALID.email, password: VALID.password };
    expect(loginSchema().safeParse(withoutFlag).success).toBe(false);
  });
});
