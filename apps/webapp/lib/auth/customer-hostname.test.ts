import { expect, it, vi } from 'vitest';

import { getCustomerHostname } from '@/lib/auth/customer-session';
import { PORTAL_SLUG_HEADER } from '@/lib/portal/routing';
import { customerPortalURL } from '@/lib/portal/server';

const request = vi.hoisted(() => ({ host: 'portal.example.com', slug: 'marko' }));
vi.mock('next/headers', () => ({
  headers: async () =>
    new Headers({
      host: request.host,
      'x-forwarded-host': 'other.example.com',
      ...(request.slug ? { [PORTAL_SLUG_HEADER]: request.slug } : {}),
    }),
}));
vi.mock('@/lib/config', () => ({
  API_URL: 'http://localhost:8000',
  PORTAL_BASE_URL: 'https://portal.example.com',
  PORTAL_BASE_HOSTNAME: 'portal.example.com',
}));

it('builds the supplier from the portal label and the configured base, whatever the Host says', async () => {
  expect(await getCustomerHostname()).toBe('marko.portal.example.com');
  // A server action's redirect re-renders through Next's own origin, which is what Host carries then.
  request.host = 'localhost:3001';
  expect(await getCustomerHostname()).toBe('marko.portal.example.com');
});

it('names no supplier outside a portal, even on a host that looks like one', async () => {
  request.slug = '';
  request.host = 'marko.portal.example.com';
  expect(await getCustomerHostname()).toBe('');
});

it('routes a delivery to its supplier portal under the configured base', () => {
  expect(customerPortalURL('marko.portal.example.com', '/quotes/t')).toBe(
    'https://portal.example.com/c/marko/quotes/t',
  );
  // A hostname outside the base has no portal here; redirecting to it would loop on the bare base.
  expect(customerPortalURL('corralon.example.org', '/quotes/t')).toBeNull();
  expect(customerPortalURL('portal.example.com', '/quotes/t')).toBeNull();
});
