import { NextRequest } from 'next/server';
import { afterEach, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { GET as callback } from '@/app/api/customer/google/callback/route';
import { GET as start } from '@/app/api/customer/google/start/route';
import { GOOGLE_SIGNUP_COOKIE } from '@/lib/auth/customer-flow';
import { PORTAL_SLUG_HEADER } from '@/lib/portal/routing';

const context = vi.hoisted(() => ({ slug: 'marko', jar: null as unknown }));
vi.mock('next/headers', () => ({
  cookies: async () => context.jar,
  headers: async () => new Headers({ host: 'portal.test', [PORTAL_SLUG_HEADER]: context.slug }),
}));
vi.mock('@/lib/config/server', () => ({ API_INTERNAL_URL: 'http://api.test' }));
afterEach(() => vi.unstubAllGlobals());

it('keeps concurrent supplier Google flows separate through the shared callback', async () => {
  context.jar = cookieJar({});
  const requests: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      requests.push(url);
      return new Response(
        JSON.stringify(
          url.endsWith('/options')
            ? {
                client_id: 'google-client',
                redirect_uri: 'https://portal.test/api/customer/google/callback',
              }
            : { signup_token: 'p'.repeat(43) },
        ),
      );
    }),
  );
  async function begin(slug: string) {
    context.slug = slug;
    const result = await start(
      new NextRequest(`https://portal.test/c/${slug}/api/customer/google/start`, {
        headers: { host: 'portal.test', [PORTAL_SLUG_HEADER]: slug },
      }),
    );
    const url = new URL(result.headers.get('location')!);
    expect(url.searchParams.get('redirect_uri')).toBe(
      'https://portal.test/api/customer/google/callback',
    );
    return url.searchParams.get('state')!;
  }
  const markoState = await begin('marko');
  await begin('other');
  context.slug = 'marko';
  const result = await callback(
    new NextRequest(
      `https://portal.test/api/customer/google/callback?code=provider-code&state=${markoState}`,
      {
        headers: { host: 'portal.test', [PORTAL_SLUG_HEADER]: 'marko' },
      },
    ),
  );
  expect(result.headers.get('location')).toBe(
    'https://portal.test/c/marko/google-complete?next=%2Fnew-request',
  );
  const jar = context.jar as ReturnType<typeof cookieJar>;
  expect(jar.get(GOOGLE_SIGNUP_COOKIE + '_marko')?.value).toBe('p'.repeat(43));
  expect(jar.get(GOOGLE_SIGNUP_COOKIE + '_other')).toBeUndefined();
  expect(requests.at(-1)).toBe(
    'http://api.test/v1/customer/sites/marko.portal.test/auth/google/exchange',
  );
  const count = requests.length;
  context.slug = 'other';
  const tampered = await callback(
    new NextRequest(
      `https://portal.test/api/customer/google/callback?code=provider-code&state=${markoState}`,
      {
        headers: { host: 'portal.test', [PORTAL_SLUG_HEADER]: 'other' },
      },
    ),
  );
  expect(tampered.headers.get('location')).toBe(
    'https://portal.test/c/other/login?error=googleFailed',
  );
  expect(requests).toHaveLength(count);
});
