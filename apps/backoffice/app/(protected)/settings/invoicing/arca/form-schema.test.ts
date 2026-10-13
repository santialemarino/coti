import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { arcaCertificateSchema } from '@/app/(protected)/settings/invoicing/arca/form-schema';
import { CERTIFICATE_MAX_BYTES } from '@/lib/constants/arca';

const BLOCK = '-----BEGIN CERTIFICATE-----\nMIIDaz\nCCAlOg==\n-----END CERTIFICATE-----';

function parse(certificate: string) {
  return arcaCertificateSchema(schemaText(true)).safeParse({ certificate });
}

describe('arcaCertificateSchema', () => {
  it('accepts one PEM block with surrounding whitespace and Windows line endings', () => {
    const result = parse(`\r\n ${BLOCK.replace(/\n/g, '\r\n')} \r\n\t`);
    expect(result.success && result.data.certificate).toBe(BLOCK);
  });

  it('tells an empty paste from a malformed one', () => {
    expect(parse('  ').error?.issues[0]?.message).toBe('field:steps.upload.required');
    for (const raw of [
      'MIIDazCCAlOg',
      BLOCK.replace('CERTIFICATE-----', 'CERTIFICATE REQUEST-----'),
      `${BLOCK}\n${BLOCK}`,
      BLOCK.replace('-----END', '----END'),
    ]) {
      expect(parse(raw).error?.issues[0]?.message).toBe('field:steps.upload.invalid');
    }
  });

  it('refuses a paste larger than the API accepts', () => {
    const huge = BLOCK.replace('MIIDaz', 'A'.repeat(CERTIFICATE_MAX_BYTES));
    expect(parse(huge).error?.issues[0]?.message).toBe('field:steps.upload.tooLarge');
  });
});
