import { expect, it } from 'vitest';

import { googlePortalSlug, portalPath } from '@/lib/portal/routing';

it('scopes navigation and intake endpoints without changing external or already scoped links', () => {
  expect(portalPath('/orders?page=2', 'marko')).toBe('/c/marko/orders?page=2');
  expect(portalPath('/api/public-rfqs/branch', 'marko')).toBe('/c/marko/api/public-rfqs/branch');
  expect(portalPath('/c/marko/orders', 'marko')).toBe('/c/marko/orders');
  expect(portalPath('https://other.test/file', 'marko')).toBe('https://other.test/file');
});

it('accepts only a well-formed supplier hint in Google state', () => {
  expect(googlePortalSlug('s'.repeat(43) + '.p.marko')).toBe('marko');
  expect(googlePortalSlug('s'.repeat(43) + '.p.other/path')).toBe('');
});
