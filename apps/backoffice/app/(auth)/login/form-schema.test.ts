import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { loginSchema } from '@/app/(auth)/login/form-schema';
import { SECRET_MAX_LENGTH } from '@/lib/constants/password';

const VALID = { email: 'admin@corralon.test', password: 'coti1234', rememberMe: false };

function messagesFor(values: Partial<typeof VALID>): Record<string, string> {
  const result = loginSchema().safeParse({ ...VALID, ...values });
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [String(issue.path[0]), issue.message]),
  );
}

describe('loginSchema', () => {
  it('accepts a complete set of credentials', () => {
    expect(loginSchema().safeParse(VALID).success).toBe(true);
  });

  // Empty and malformed are different rejections, and telling the caller which one they hit is
  // the whole point of checking presence before format.
  it('reports an empty address as missing, not as malformed', () => {
    expect(messagesFor({ email: '' }).email).toBe('email.required');
    expect(messagesFor({ email: '   ' }).email).toBe('email.required');
  });

  it.each(['no-at-sign', 'missing@tld', '@corralon.test'])('reports %p as malformed', (email) => {
    expect(messagesFor({ email }).email).toBe('invalidEmail');
  });

  it('reports an empty password as missing', () => {
    expect(messagesFor({ password: '' }).password).toBe('password.required');
  });

  /*
   * No floor here on purpose: this password is being presented, not chosen, so a rule that grew
   * after an account was created must not lock that account out of its own login screen.
   */
  it('accepts a password shorter than the policy for a new one', () => {
    expect(loginSchema().safeParse({ ...VALID, password: 'abc' }).success).toBe(true);
  });

  it('refuses a password past the length the API accepts', () => {
    expect(messagesFor({ password: 'a'.repeat(SECRET_MAX_LENGTH + 1) }).password).toBe(
      'passwordTooLong',
    );
  });

  /*
   * The factory takes the translators so a message is a catalog key rather than Spanish baked
   * into the schema — which is what lets the server action re-validate with the same schema
   * and ignore the wording. A field's own message and a shared one come from different catalogs.
   */
  it('resolves each message through the catalog it belongs to', () => {
    const schema = loginSchema(schemaText(true));

    expect(schema.safeParse({ ...VALID, email: '' }).error?.issues[0]?.message).toBe(
      'field:email.required',
    );
    expect(schema.safeParse({ ...VALID, email: 'nope' }).error?.issues[0]?.message).toBe(
      'shared:invalidEmail',
    );
  });

  it('requires rememberMe to be present, since the session length depends on it', () => {
    expect(loginSchema().safeParse({ email: VALID.email, password: VALID.password }).success).toBe(
      false,
    );
  });
});
