import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { TEXT_FIELD_MAX_LENGTH, URL_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

const VALID: AccountValues = {
  name: 'Corralón San Martín',
  legalName: '',
  taxId: '',
  brandLogoUrl: '',
  brandColor: '',
};

function messagesFor(values: Partial<AccountValues>) {
  const result = accountSchema().safeParse({ ...VALID, ...values });
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [String(issue.path[0]), issue.message]),
  );
}

describe('accountSchema', () => {
  // Everything but the name is optional, and an unset optional field is the empty string a text
  // input holds — not an absent key.
  it('accepts a corralón with nothing but a name', () => {
    expect(accountSchema().safeParse(VALID).success).toBe(true);
  });

  it('trims the name it accepts', () => {
    const parsed = accountSchema().safeParse({ ...VALID, name: '  Corralón San Martín  ' });

    expect(parsed.success && parsed.data.name).toBe('Corralón San Martín');
  });

  it('refuses a name of nothing but spaces', () => {
    expect(messagesFor({ name: '   ' }).name).toBe('name.required');
  });

  it('refuses a name past the length the API stores', () => {
    expect(messagesFor({ name: 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1) }).name).toBe('tooLong');
  });

  it('refuses a legal name or a tax id past that length', () => {
    const tooLong = 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1);

    expect(messagesFor({ legalName: tooLong }).legalName).toBe('tooLong');
    expect(messagesFor({ taxId: tooLong }).taxId).toBe('tooLong');
  });

  it('resolves each message through the catalog it belongs to', () => {
    const schema = accountSchema(schemaText(true));

    expect(schema.safeParse({ ...VALID, name: '' }).error?.issues[0]?.message).toBe(
      'field:name.required',
    );
    expect(
      schema.safeParse({ ...VALID, legalName: 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1) }).error
        ?.issues[0]?.message,
    ).toBe('shared:tooLong');
  });
});

/*
 * The brand formats mirror what the API accepts, neither looser nor stricter. Stricter is the
 * dangerous direction: the form would refuse a value already in the column and the account could
 * never save anything again.
 */
describe('accountSchema and the brand', () => {
  it.each(['#C2410C', '#c2410c', '#FFF', '#ffff', '#C2410C80'])('accepts the colour %s', (raw) => {
    expect(accountSchema().safeParse({ ...VALID, brandColor: raw }).success).toBe(true);
  });

  it.each(['C2410C', 'naranja', '#C2410', '#', 'rgb(194, 65, 12)', '#GGGGGG'])(
    'refuses the colour %p',
    (raw) => {
      expect(messagesFor({ brandColor: raw }).brandColor).toBe('brandColor.invalid');
    },
  );

  it.each([
    'https://tucorralon.com/logo.png',
    'http://localhost:3000/logo.svg',
    'https://cdn.example.test/a/b/c.png?v=2',
  ])('accepts the logo %s', (raw) => {
    expect(accountSchema().safeParse({ ...VALID, brandLogoUrl: raw }).success).toBe(true);
  });

  // A schemeless address is what someone pastes from a browser's address bar, and the API refuses
  // it too — a base with no scheme resolves to nothing when the webapp renders it.
  it.each(['tucorralon.com/logo.png', '/logo.png', 'logo.png', 'www.tucorralon.com'])(
    'refuses the logo %p',
    (raw) => {
      expect(messagesFor({ brandLogoUrl: raw }).brandLogoUrl).toBe('brandLogoUrl.invalid');
    },
  );

  it('refuses a logo past the length the API stores', () => {
    const tooLong = `https://tucorralon.com/${'a'.repeat(URL_FIELD_MAX_LENGTH)}.png`;

    // Its own message, because the cap is 512 rather than the 255 every other field shares — and a
    // schema message cannot interpolate, so each number is baked into its own key.
    expect(messagesFor({ brandLogoUrl: tooLong }).brandLogoUrl).toBe('tooLong');
  });

  // Empty is how the brand is cleared, so neither format check may reject it.
  it('accepts an empty colour and an empty logo', () => {
    expect(accountSchema().safeParse({ ...VALID, brandColor: '', brandLogoUrl: '' }).success).toBe(
      true,
    );
  });
});
