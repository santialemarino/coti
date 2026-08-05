import { describe, expect, it } from 'vitest';

import { userSchema, type UserValues } from '@/app/(protected)/settings/users/form-schema';
import { ADMIN_ROLE, PASSWORD_MAX_LENGTH, PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

const VALID: UserValues = {
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: ADMIN_ROLE,
  branchIds: [],
  password: 'coti1234',
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

  it.each(['', 'ana', 'ana@', '@corralon.test', 'ana corralon.test'])(
    'refuses %p as an address',
    (email) => {
      expect(messagesFor({ email }).email).toBe('email.invalid');
    },
  );

  it('refuses a role outside the two the interface offers', () => {
    expect(messagesFor({ role: 'OWNER' as UserValues['role'] }).role).toBe('role.required');
  });

  it('resolves every message through the translator it is given', () => {
    const parsed = userSchema('create', (key) => `t:${key}`).safeParse({ ...VALID, name: '' });

    expect(parsed.error?.issues[0]?.message).toBe('t:name.required');
  });
});

/*
 * The mode decides one field. A password is set once, at creation: the API's update body carries
 * none, so requiring one to edit a profile would make every edit impossible.
 */
describe('userSchema and the initial password', () => {
  it.each(['', 'corto', 'a'.repeat(PASSWORD_MIN_LENGTH - 1)])(
    'refuses %p when creating',
    (password) => {
      expect(messagesFor({ password }).password).toBe('password.tooShort');
    },
  );

  // bcrypt hashes the first 72 bytes and ignores the rest, so the API refuses a longer one rather
  // than accepting a password whose tail never mattered.
  it('refuses a password past what the API stores', () => {
    expect(messagesFor({ password: 'a'.repeat(PASSWORD_MAX_LENGTH + 1) }).password).toBe(
      'password.tooLong',
    );
  });

  it('accepts a password of exactly the minimum length', () => {
    const password = 'a'.repeat(PASSWORD_MIN_LENGTH);

    expect(userSchema('create').safeParse({ ...VALID, password }).success).toBe(true);
  });

  it('asks for none when editing', () => {
    expect(userSchema('edit').safeParse({ ...VALID, password: '' }).success).toBe(true);
  });
});
