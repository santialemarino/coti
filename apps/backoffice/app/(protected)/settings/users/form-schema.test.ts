import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { userSchema, type UserValues } from '@/app/(protected)/settings/users/form-schema';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { PASSWORD_MAX_BYTES, PASSWORD_MIN_LENGTH } from '@/lib/constants/password';

const VALID: UserValues = {
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: ADMIN_ROLE,
  branchIds: [],
  password: 'Coti-1234-larga',
};

function messagesFor(values: Partial<UserValues>, mode: 'create' | 'edit' = 'create') {
  const result = userSchema(mode).safeParse({ ...VALID, ...values });
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [String(issue.path[0]), issue.message]),
  );
}

describe('userSchema', () => {
  it('accepts a user with no branches assigned', () => {
    expect(userSchema('create').safeParse(VALID).success).toBe(true);
  });

  it('trims the name it accepts', () => {
    const parsed = userSchema('create').safeParse({ ...VALID, name: '  Ana Gómez  ' });

    expect(parsed.success && parsed.data.name).toBe('Ana Gómez');
  });

  // Trimmed before the check, or a name of nothing but spaces reaches the API as a blank the
  // listing cannot render.
  it('refuses a name of nothing but spaces', () => {
    expect(messagesFor({ name: '   ' }).name).toBe('name.required');
  });

  it.each([TEXT_FIELD_MAX_LENGTH + 1])('refuses a name of %i characters', (length) => {
    expect(messagesFor({ name: 'a'.repeat(length) }).name).toBe('tooLong');
  });

  it.each(['ana', 'ana@', '@corralon.test', 'ana corralon.test'])(
    'reports %p as a malformed address',
    (email) => {
      expect(messagesFor({ email }).email).toBe('invalidEmail');
    },
  );

  it('refuses a role outside the two the interface offers', () => {
    expect(messagesFor({ role: 'OWNER' as UserValues['role'] }).role).toBe('role.required');
  });

  it('resolves each message through the catalog it belongs to', () => {
    const schema = userSchema('create', schemaText(true));

    expect(schema.safeParse({ ...VALID, name: '' }).error?.issues[0]?.message).toBe(
      'field:name.required',
    );
    expect(schema.safeParse({ ...VALID, email: 'nope' }).error?.issues[0]?.message).toBe(
      'shared:invalidEmail',
    );
  });

  // Empty and malformed are different rejections, and an admin filling this in needs to know which.
  it.each([
    ['email', '', 'email.required'],
    ['password', '', 'password.required'],
  ] as const)('reports an empty %s as missing', (field, value, message) => {
    expect(messagesFor({ [field]: value })[field]).toBe(message);
  });
});

/*
 * The mode decides one field. A password is set once, at creation: the API's update body carries
 * none, so requiring one to edit a profile would make every edit impossible.
 */
describe('userSchema and the initial password', () => {
  it.each(['Aa1!bcd', `Aa1!${'b'.repeat(PASSWORD_MIN_LENGTH - 5)}`])(
    'refuses %p when creating',
    (password) => {
      expect(messagesFor({ password }).password).toBe('passwordTooShort');
    },
  );

  it('refuses a long password that is missing a character class', () => {
    expect(messagesFor({ password: 'contraseña-larga' }).password).toBe('passwordRequirements');
  });

  // bcrypt hashes the first 72 bytes and ignores the rest, so the API refuses a longer one rather
  // than accepting a password whose tail never mattered.
  it('refuses a password past what the API stores', () => {
    expect(messagesFor({ password: `Aa1!${'b'.repeat(PASSWORD_MAX_BYTES)}` }).password).toBe(
      'passwordTooLong',
    );
  });

  it('accepts a password of exactly the minimum length', () => {
    const password = `Aa1!${'b'.repeat(PASSWORD_MIN_LENGTH - 4)}`;

    expect(userSchema('create').safeParse({ ...VALID, password }).success).toBe(true);
  });

  it('asks for none when editing', () => {
    expect(userSchema('edit').safeParse({ ...VALID, password: '' }).success).toBe(true);
  });
});
