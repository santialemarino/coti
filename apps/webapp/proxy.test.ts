import { NextRequest } from 'next/server';
import { expect, it } from 'vitest';

import { LANDING_URL } from '@/lib/config';
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

it.each(['/', '/login', '/orders/123', '/c', '/c/', '/signup?next=%2Forders', '/quotes/a/b'])(
  'sends %s, which names no portal, straight to the landing',
  (path) => {
    const result = proxy(
      new NextRequest('https://portal.test' + path, { headers: { [PORTAL_SLUG_HEADER]: 'other' } }),
    );
    expect(result.status).toBe(307);
    expect(result.headers.get('location')).toBe(new URL(LANDING_URL).href);
  },
);

it.each(['/quotes/legacy-token', '/api/public-rfqs/channel', '/api/customer/google/start'])(
  'lets the unscoped entry point %s through without a supplier',
  (path) => {
    const result = proxy(
      new NextRequest('https://portal.test' + path, { headers: { [PORTAL_SLUG_HEADER]: 'other' } }),
    );
    expect(result.headers.get('x-middleware-next')).toBe('1');
    expect(result.headers.has('x-middleware-request-' + PORTAL_SLUG_HEADER)).toBe(false);
  },
);

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
