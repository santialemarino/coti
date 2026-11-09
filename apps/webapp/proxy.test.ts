import { NextRequest } from 'next/server';
import { expect, it } from 'vitest';

import { proxy } from '@/lib/portal/proxy';
import { PORTAL_SLUG_HEADER } from '@/lib/portal/routing';

it('takes the supplier only from the public path and overwrites caller headers', () => {
  const result = proxy(
    new NextRequest('https://portal.test/c/marko/orders?page=2', {
      headers: { [PORTAL_SLUG_HEADER]: 'other' },
    }),
  );
  expect(result.headers.get('x-middleware-rewrite')).toBe('https://portal.test/orders?page=2');
  expect(result.headers.get('x-middleware-request-' + PORTAL_SLUG_HEADER)).toBe('marko');
});

it('removes supplier header spoofing from unscoped requests', () => {
  const result = proxy(
    new NextRequest('https://portal.test/login', {
      headers: { [PORTAL_SLUG_HEADER]: 'other' },
    }),
  );
  expect(result.headers.has('x-middleware-request-' + PORTAL_SLUG_HEADER)).toBe(false);
});

it.each(['/c/Marko/login', '/c/-marko/login', '/c/marko/c/other/orders'])(
  'rejects an invalid or nested supplier path %s',
  (path) => {
    expect(proxy(new NextRequest('https://portal.test' + path)).status).toBe(404);
  },
);

it('restores the supplier on the one shared Google callback without trusting arbitrary state', () => {
  const url = 'https://portal.test/api/customer/google/callback?state=';
  const result = proxy(new NextRequest(url + 's'.repeat(43) + '.p.marko'));
  expect(result.headers.get('x-middleware-request-' + PORTAL_SLUG_HEADER)).toBe('marko');
  const invalid = proxy(new NextRequest(url + 'arbitrary.p.other'));
  expect(invalid.headers.has('x-middleware-request-' + PORTAL_SLUG_HEADER)).toBe(false);
});
