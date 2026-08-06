import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { PASSWORD_MAX_LENGTH } from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

const VALID: SignupValues = {
  accountName: 'Corralón San Martín',
  legalName: '',
  taxId: '',
  branchName: 'Villa Bosch',
  branchAddress: '',
  adminName: 'Ana Pérez',
  adminEmail: 'ana@corralonsanmartin.test',
  adminPassword: 'coti1234',
  confirmPassword: 'coti1234',
};

function messagesFor(values: Partial<SignupValues>): Record<string, string> {
  const result = signupSchema().safeParse({ ...VALID, ...values });
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [String(issue.path[0]), issue.message]),
  );
}

describe('signupSchema', () => {
  it('accepts a registration with every optional field left blank', () => {
    expect(signupSchema().safeParse(VALID).success).toBe(true);
  });

  // The three fiscal and address fields are genuinely optional, so a blank one must not be
  // an error — the action is what turns it into an omitted key.
  it('trims what it accepts, so the action never sees padding the caller did not mean', () => {
    const parsed = signupSchema().safeParse({ ...VALID, accountName: '  Corralón  ' });

    expect(parsed.success && parsed.data.accountName).toBe('Corralón');
  });

  /*
   * The reason `trim()` sits before `min(1)` rather than after: a name of nothing but spaces
   * passes any length check on the raw string, and the API would store it as a blank name
   * that no screen can show.
   */
  it.each(['accountName', 'branchName', 'adminName'] as const)(
    'refuses a %s of nothing but spaces',
    (field) => {
      expect(messagesFor({ [field]: '   ' })[field]).toBe(`${field}.required`);
    },
  );

  it('refuses a text field past the length the API stores', () => {
    expect(messagesFor({ accountName: 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1) }).accountName).toBe(
      'tooLong',
    );
  });

  it('refuses a password past the length bcrypt reads', () => {
    expect(messagesFor({ adminPassword: 'a'.repeat(PASSWORD_MAX_LENGTH + 1) }).adminPassword).toBe(
      'passwordTooLong',
    );
  });

  it('refuses a short password', () => {
    expect(messagesFor({ adminPassword: 'short', confirmPassword: 'short' }).adminPassword).toBe(
      'passwordTooShort',
    );
  });

  // Empty and malformed are different rejections, on every field that has a format.
  it.each([
    ['adminEmail', '', 'adminEmail.required'],
    ['adminEmail', 'ana@', 'invalidEmail'],
    ['adminPassword', '', 'adminPassword.required'],
    ['confirmPassword', '', 'confirmPassword.required'],
  ] as const)('reports %s of %p as %s', (field, value, message) => {
    expect(messagesFor({ [field]: value })[field]).toBe(message);
  });

  it('reports a mistyped confirmation on the confirmation field', () => {
    expect(messagesFor({ confirmPassword: 'coti12345' }).confirmPassword).toBe('passwordMismatch');
  });

  // The messages are catalog keys the form resolves, never strings baked into the schema.
  it('resolves each message through the catalog it belongs to', () => {
    const schema = signupSchema(schemaText(true));

    expect(schema.safeParse({ ...VALID, accountName: '' }).error?.issues[0]?.message).toBe(
      'field:accountName.required',
    );
    expect(schema.safeParse({ ...VALID, adminEmail: 'ana@' }).error?.issues[0]?.message).toBe(
      'shared:invalidEmail',
    );
  });
});
