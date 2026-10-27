import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  CUSTOMER_SESSION_COOKIE,
  CustomerAuthError,
  endCustomerSession,
  getCustomerSession,
  startCustomerSession,
} from '@/lib/auth/customer-session';

const jar = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn() }));

vi.mock('next/headers', () => ({
  cookies: async () => jar,
  headers: async () =>
    new Headers({ host: 'supplier.coti.test:3001', 'x-forwarded-host': 'other.coti.test' }),
}));

const identity = {
  id: 'customer-id',
  tax_id: '20123456786',
  name: null,
  session_expires_at: '2030-10-19T12:00:00Z',
};
const secret = 'A'.repeat(43);

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe('customer sessions', () => {
  it('persists only a host-only HttpOnly cookie and returns no secret to client callers', async () => {
    const request = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ session_token: secret, customer: identity }), {
        status: 201,
      }),
    );
    vi.stubGlobal('fetch', request);

    const session = await startCustomerSession('register', identity.tax_id, 'Customer-pass123!');

    expect(session).toEqual({
      customerId: identity.id,
      taxId: identity.tax_id,
      name: null,
      sessionExpiresAt: identity.session_expires_at,
    });
    expect(request.mock.calls[0]?.[0]).toContain(
      '/v1/customer/sites/supplier.coti.test/auth/register',
    );
    expect(JSON.parse(request.mock.calls[0]?.[1].body)).toEqual({
      tax_id: identity.tax_id,
      password: 'Customer-pass123!',
    });
    expect(jar.set).toHaveBeenCalledWith(
      CUSTOMER_SESSION_COOKIE,
      secret,
      expect.objectContaining({
        httpOnly: true,
        sameSite: 'lax',
        path: '/',
        expires: new Date(identity.session_expires_at),
      }),
    );
    expect(jar.set.mock.calls[0]?.[2]).not.toHaveProperty('domain');
  });

  it('verifies the session with the API instead of trusting cookie presence', async () => {
    jar.get.mockReturnValue({ value: secret });
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    vi.stubGlobal('fetch', request);
    await expect(getCustomerSession()).resolves.toBeNull();
    expect(request).toHaveBeenCalledWith(
      expect.stringContaining('/supplier.coti.test/me'),
      expect.objectContaining({
        cache: 'no-store',
        headers: { Authorization: `Bearer ${secret}` },
      }),
    );
  });

  it('does not store a failed or expired login', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    await expect(startCustomerSession('login', identity.tax_id, 'bad')).rejects.toBeInstanceOf(
      CustomerAuthError,
    );
    expect(jar.set).not.toHaveBeenCalled();

    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            session_token: secret,
            customer: { ...identity, session_expires_at: '2000-01-01T00:00:00Z' },
          }),
        ),
      ),
    );
    await expect(
      startCustomerSession('login', identity.tax_id, 'Customer-pass123!'),
    ).rejects.toMatchObject({ status: 502 });
    expect(jar.set).not.toHaveBeenCalled();
  });

  it('revokes the server session and expires the local cookie when logging out', async () => {
    jar.get.mockReturnValue({ value: secret });
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', request);
    await endCustomerSession();
    expect(request).toHaveBeenCalledWith(
      expect.stringContaining('/supplier.coti.test/auth/logout'),
      expect.objectContaining({ method: 'POST', headers: { Authorization: `Bearer ${secret}` } }),
    );
    expect(jar.set).toHaveBeenCalledWith(
      CUSTOMER_SESSION_COOKIE,
      '',
      expect.objectContaining({ httpOnly: true, expires: new Date(0) }),
    );
  });
});
