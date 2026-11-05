import { describe, expect, it } from 'vitest';

import { portalPreview, portalSchema } from '@/components/customer-portal/form-schema';

describe('portal input', () => {
  it('normalizes a label while refusing URLs, nested names and malformed DNS labels', () => {
    expect(portalSchema().parse({ subdomain: ' Marko ' })).toEqual({ subdomain: 'marko' });
    for (const subdomain of ['', '-x', 'x-', 'a.b', 'https://x', 'corralón', 'a'.repeat(64)])
      expect(portalSchema().safeParse({ subdomain }).success).toBe(false);
  });
  it('uses the configured scheme, base hostname and port', () => {
    expect(portalPreview('http://localhost:3001', 'Marko')).toBe('http://marko.localhost:3001');
    expect(portalPreview('https://suppliers.test', 'Marko')).toBe('https://marko.suppliers.test');
    expect(portalPreview('', 'Marko')).toBe('');
  });
});
