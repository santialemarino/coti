import { describe, expect, it } from 'vitest';

import { whatsappChannelSchema } from '@/app/(protected)/settings/branches/[branchId]/channels/form-schema';

const VALUES = {
  identifier: '+5491155550101',
  phoneNumberId: '1379080161952101',
  businessAccountId: '3580903055408468',
  accessToken: 'x'.repeat(400),
  webhookVerifyToken: 'test-verifier',
  isActive: true,
};

describe('whatsappChannelSchema', () => {
  it('preserves blank credentials on an already configured channel', () => {
    expect(
      whatsappChannelSchema(undefined, true, VALUES.identifier).safeParse({
        ...VALUES,
        phoneNumberId: '',
        businessAccountId: '',
        accessToken: '',
        webhookVerifyToken: '',
      }).success,
    ).toBe(true);
  });

  it('requires provider references and access credentials when switching the number', () => {
    const changed = {
      ...VALUES,
      identifier: '+541143210000',
      phoneNumberId: '',
      businessAccountId: '',
      accessToken: '',
    };
    expect(
      whatsappChannelSchema(undefined, true, VALUES.identifier).safeParse(changed).success,
    ).toBe(false);
    expect(
      whatsappChannelSchema(undefined, true, VALUES.identifier).safeParse({
        ...VALUES,
        identifier: '+541143210000',
        webhookVerifyToken: '',
      }).success,
    ).toBe(true);
  });

  it('keeps both real Argentine mobile and landline numbers unchanged', () => {
    for (const identifier of ['+5491155550101', '+541155550101']) {
      expect(whatsappChannelSchema().parse({ ...VALUES, identifier }).identifier).toBe(identifier);
    }
  });

  it.each(['+5491155550101', '+15556530731'])(
    'accepts a real or Meta test number: %s',
    (identifier) => {
      expect(whatsappChannelSchema().safeParse({ ...VALUES, identifier }).success).toBe(true);
    },
  );

  it.each(['', '+1555', 'not-a-phone'])('rejects an incomplete number: %s', (identifier) => {
    expect(whatsappChannelSchema().safeParse({ ...VALUES, identifier }).success).toBe(false);
  });

  it('accepts provider tokens longer than account passwords without truncation', () => {
    expect(whatsappChannelSchema().parse(VALUES).accessToken).toHaveLength(400);
  });

  it.each(['phoneNumberId', 'businessAccountId'] as const)(
    'bounds the provider identifier %s to the API limit',
    (field) => {
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: '1'.repeat(255) }).success,
      ).toBe(true);
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: '1'.repeat(256) }).success,
      ).toBe(false);
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: 'not-a-meta-id' }).success,
      ).toBe(false);
    },
  );

  it.each(['accessToken', 'webhookVerifyToken'] as const)(
    'bounds %s to the API credential limit',
    (field) => {
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: 'a'.repeat(4096) }).success,
      ).toBe(true);
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: 'a'.repeat(4097) }).success,
      ).toBe(false);
    },
  );

  it.each(['accessToken', 'webhookVerifyToken'] as const)(
    'applies the API byte limit to multibyte characters in %s',
    (field) => {
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: 'ñ'.repeat(2048) }).success,
      ).toBe(true);
      expect(
        whatsappChannelSchema().safeParse({ ...VALUES, [field]: 'ñ'.repeat(2049) }).success,
      ).toBe(false);
    },
  );

  it.each(['phoneNumberId', 'businessAccountId', 'accessToken', 'webhookVerifyToken'] as const)(
    'requires %s on a complete replacement',
    (field) => {
      expect(whatsappChannelSchema().safeParse({ ...VALUES, [field]: '' }).success).toBe(false);
    },
  );
});
