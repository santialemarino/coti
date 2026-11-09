import { expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import {
  CUSTOMER_SESSION_COOKIE,
  getCustomerHostname,
  getCustomerToken,
} from '@/lib/auth/customer-session';
import { customerCookies } from '@/lib/portal/cookies';
import { PORTAL_SLUG_HEADER } from '@/lib/portal/routing';

const context = vi.hoisted(() => ({ slug: 'marko', jar: null as unknown }));
vi.mock('next/headers', () => ({
  cookies: async () => context.jar,
  headers: async () => new Headers({ host: 'portal.test', [PORTAL_SLUG_HEADER]: context.slug }),
}));

it('reads, writes and clears cookies independently for two suppliers on the same host', async () => {
  const jar = cookieJar({
    [CUSTOMER_SESSION_COOKIE + '_marko']: 'session-a',
    [CUSTOMER_SESSION_COOKIE + '_other']: 'session-b',
  });
  context.jar = jar;
  context.slug = 'marko';
  expect(await getCustomerToken()).toBe('session-a');
  expect(await getCustomerHostname()).toBe('marko.portal.test');
  (await customerCookies()).set(CUSTOMER_SESSION_COOKIE, '', { path: '/', expires: new Date(0) });
  expect(await getCustomerToken()).toBeUndefined();
  context.slug = 'other';
  expect(await getCustomerToken()).toBe('session-b');
  expect(await getCustomerHostname()).toBe('other.portal.test');
  context.slug = 'unknown';
  expect(await getCustomerToken()).toBeUndefined();
});
