import { describe, expect, it } from 'vitest';

import { customerProfileSchema } from '@/app/profile/form-schema';

const schema = customerProfileSchema((key) => key);

describe('customerProfileSchema', () => {
  it('requires a full name and at least one completed contact', () => {
    expect(schema.safeParse({ name: '', contacts: [] }).success).toBe(false);
    expect(
      schema.safeParse({
        name: 'Customer',
        contacts: [{ kind: 'EMAIL', value: '', primary: true }],
      }).success,
    ).toBe(false);
    expect(
      schema.safeParse({
        name: 'Juan Perez',
        contacts: [
          { kind: 'EMAIL', value: 'one@example.test', primary: true },
          { kind: 'WHATSAPP', value: '', primary: true },
        ],
      }).success,
    ).toBe(true);
  });
  it('accepts many contacts without an expected count or a business cap', () => {
    const contacts = Array.from({ length: 150 }, (_, index) => ({
      kind: 'EMAIL',
      value: `customer${index}@example.test`,
      primary: index === 0,
    }));
    expect(schema.safeParse({ name: 'Customer', contacts }).success).toBe(true);
  });
  it('rejects case-insensitive duplicates at the second field', () => {
    const result = schema.safeParse({
      name: 'Customer',
      contacts: [
        { kind: 'EMAIL', value: 'one@example.test', primary: true },
        { kind: 'EMAIL', value: 'ONE@EXAMPLE.TEST', primary: false },
      ],
    });
    expect(result.success).toBe(false);
    if (!result.success)
      expect(result.error.issues).toContainEqual(
        expect.objectContaining({ path: ['contacts', 1, 'value'], message: 'duplicate' }),
      );
  });
  it('asks for a replacement when the primary contact was removed', () => {
    const result = schema.safeParse({
      name: 'Customer',
      contacts: [{ kind: 'EMAIL', value: 'one@example.test', primary: false }],
    });
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues[0]?.message).toBe('choosePrimary');
  });
  it.each(['EMAIL', 'WHATSAPP'])('validates malformed %s fields', (kind) => {
    const result = schema.safeParse({
      name: 'Customer',
      contacts: [{ kind, value: 'bad', primary: true }],
    });
    expect(result.success).toBe(false);
  });
});
