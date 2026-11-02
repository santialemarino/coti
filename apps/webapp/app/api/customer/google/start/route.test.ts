import { cookies } from 'next/headers';
import { NextRequest } from 'next/server';
import { afterEach, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { GET } from '@/app/api/customer/google/start/route';
import { GOOGLE_STATE_COOKIE } from '@/lib/auth/customer-flow';
import { localGoogleReturnOrigin } from '@/lib/auth/local-google-flow';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('@/lib/auth/customer-session', () => ({
  customerApiPath: async () => 'http://api.test/options',
}));
afterEach(() => vi.unstubAllGlobals());

it('keeps state, nonce and PKCE in the starting supplier cookie while using the shared callback', async () => {
  const jar = cookieJar({});
  vi.mocked(cookies).mockResolvedValue(jar as unknown as Awaited<ReturnType<typeof cookies>>);
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          client_id: 'development-client',
          redirect_uri: 'http://localhost:3001/api/customer/google/callback',
        }),
      ),
    ),
  );
  const response = await GET(
    new NextRequest('http://sanmartin.localhost:3001/api/customer/google/start'),
  );
  const google = new URL(response.headers.get('location')!);
  const flow = JSON.parse(jar.get(GOOGLE_STATE_COOKIE)!.value);
  expect(google.origin).toBe('https://accounts.google.com');
  expect(google.searchParams.get('redirect_uri')).toBe(
    'http://localhost:3001/api/customer/google/callback',
  );
  expect(google.searchParams.get('state')).toBe(flow.state);
  expect(localGoogleReturnOrigin(flow.state, 'http://localhost:3001')).toBe(
    'http://sanmartin.localhost:3001',
  );
  expect(flow.verifier).toHaveLength(43);
  expect(flow.nonce).toHaveLength(43);
  expect(google.searchParams.get('code_challenge_method')).toBe('S256');
  const options = jar.set.mock.calls[0]![2];
  expect(options).toMatchObject({ httpOnly: true, sameSite: 'lax', path: '/' });
  expect(options).not.toHaveProperty('domain');
});
