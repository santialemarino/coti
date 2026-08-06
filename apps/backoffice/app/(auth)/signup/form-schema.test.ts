import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { PASSWORD_MAX_BYTES } from '@/lib/constants/password';

const VALID: SignupValues = {
  accountName: 'Corralón San Martín',
  legalName: '',
  taxId: '',
  branchName: 'Villa Bosch',
  branchAddress: '',
  adminName: 'Ana Pérez',
  adminEmail: 'ana@corralonsanmartin.test',
  adminPassword: 'Coti-1234-larga',
  confirmPassword: 'Coti-1234-larga',
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
    const past = `Aa1!${'b'.repeat(PASSWORD_MAX_BYTES)}`;

    expect(messagesFor({ adminPassword: past, confirmPassword: past }).adminPassword).toBe(
      'passwordTooLong',
    );
  });

  it('refuses a short password even when it carries every character class', () => {
    expect(
      messagesFor({ adminPassword: 'Aa1!bcd', confirmPassword: 'Aa1!bcd' }).adminPassword,
    ).toBe('passwordTooShort');
  });

  // Long enough is not enough: the policy the API applies is mirrored here, class by class.
  it.each(['contraseña-larga1', 'CONTRASEÑA-LARGA1', 'Contraseña-larga', 'Contrasena1larga'])(
    'refuses %p for the class it is missing',
    (adminPassword) => {
      expect(messagesFor({ adminPassword, confirmPassword: adminPassword }).adminPassword).toBe(
        'passwordRequirements',
      );
    },
  );

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
    expect(messagesFor({ confirmPassword: 'Coti-1234-larga!' }).confirmPassword).toBe(
      'passwordMismatch',
    );
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
