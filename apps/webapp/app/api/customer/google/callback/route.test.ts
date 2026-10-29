import { cookies } from 'next/headers';
import { NextRequest } from 'next/server';
import { beforeEach, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { GET } from '@/app/api/customer/google/callback/route';
import {
  GOOGLE_EMAIL_COOKIE,
  GOOGLE_SIGNUP_COOKIE,
  GOOGLE_STATE_COOKIE,
  postCustomerAuth,
} from '@/lib/auth/customer-flow';
import { acceptCustomerSession } from '@/lib/auth/customer-session';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('@/lib/auth/customer-flow', async (original) => ({
  ...(await original<typeof import('@/lib/auth/customer-flow')>()),
  postCustomerAuth: vi.fn(),
}));
vi.mock('@/lib/auth/customer-session', async (original) => ({
  ...(await original<typeof import('@/lib/auth/customer-session')>()),
  acceptCustomerSession: vi.fn(),
}));

const state = 's'.repeat(43);
function flow(next = '/orders', expiresAt = Date.now() + 60000) {
  const jar = cookieJar({
    [GOOGLE_STATE_COOKIE]: JSON.stringify({
      state,
      nonce: 'n'.repeat(43),
      verifier: 'v'.repeat(43),
      next,
      expiresAt,
    }),
  });
  vi.mocked(cookies).mockResolvedValue(jar as unknown as Awaited<ReturnType<typeof cookies>>);
  return jar;
}
function callback(value = state) {
  return new NextRequest(
    `https://supplier.test/api/customer/google/callback?code=one-use-code&state=${value}`,
  );
}
beforeEach(() => vi.clearAllMocks());

it('rejects a mismatched or expired browser state before exchanging any code', async () => {
  flow();
  const denied = await GET(callback('wrong-state'));
  expect(denied.headers.get('location')).toContain('/login?error=googleFailed');
  expect(postCustomerAuth).not.toHaveBeenCalled();
  flow('/orders', Date.now() - 1);
  await GET(callback());
  expect(postCustomerAuth).not.toHaveBeenCalled();
  expect(acceptCustomerSession).not.toHaveBeenCalled();
});

it('keeps a first-time Google identity in a short-lived HttpOnly cookie until CUIT/CUIL is supplied', async () => {
  const jar = flow('/quotes/existing-delivery');
  vi.mocked(postCustomerAuth).mockResolvedValue(
    new Response(JSON.stringify({ signup_token: 'p'.repeat(43) })),
  );
  const response = await GET(callback());
  expect(postCustomerAuth).toHaveBeenCalledWith('/auth/google/exchange', {
    code: 'one-use-code',
    verifier: 'v'.repeat(43),
    nonce: 'n'.repeat(43),
  });
  expect(response.headers.get('location')).toBe(
    'https://supplier.test/google-complete?next=%2Fquotes%2Fexisting-delivery',
  );
  expect(jar.set).toHaveBeenCalledWith(
    GOOGLE_SIGNUP_COOKIE,
    'p'.repeat(43),
    expect.objectContaining({ httpOnly: true, sameSite: 'lax', path: '/' }),
  );
  expect(jar.set).toHaveBeenCalledWith(
    GOOGLE_EMAIL_COOKIE,
    '',
    expect.objectContaining({ expires: new Date(0), path: '/' }),
  );
  expect(acceptCustomerSession).not.toHaveBeenCalled();
  await GET(callback());
  expect(postCustomerAuth).toHaveBeenCalledTimes(1);
});

it('keeps the Google-verified address to offer it as an optional contact', async () => {
  const jar = flow();
  vi.mocked(postCustomerAuth).mockResolvedValue(
    new Response(JSON.stringify({ signup_token: 'p'.repeat(43), signup_email: 'juan@gmail.com' })),
  );
  await GET(callback());
  expect(jar.set).toHaveBeenCalledWith(
    GOOGLE_EMAIL_COOKIE,
    'juan@gmail.com',
    expect.objectContaining({ httpOnly: true, sameSite: 'lax', path: '/' }),
  );
});

it('starts a known account session and refuses an external post-login destination', async () => {
  flow('https://attacker.test');
  const session = {
    session_token: 'a'.repeat(43),
    customer: {
      id: 'customer',
      tax_id: '20123456786',
      name: null,
      session_expires_at: '2030-01-01T00:00:00Z',
    },
  };
  vi.mocked(postCustomerAuth).mockResolvedValue(new Response(JSON.stringify({ session })));
  const response = await GET(callback());
  expect(acceptCustomerSession).toHaveBeenCalledWith(session);
  expect(response.headers.get('location')).toBe('https://supplier.test/orders');
});
