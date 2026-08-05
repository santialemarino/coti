import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, apiRequest, errorCodeOf } from '@/lib/api/client';

vi.mock('@/lib/auth/session', () => ({ getAccessToken: vi.fn() }));
vi.mock('@/lib/auth/client-address', () => ({ clientAddress: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getActiveBranchId: vi.fn() }));

const { getAccessToken } = await import('@/lib/auth/session');
const { clientAddress } = await import('@/lib/auth/client-address');
const { getActiveBranchId } = await import('@/lib/auth/branch');

const TOKEN = 'access-token';

// A fresh Response per call: a body can only be read once, and several cases fetch twice.
function responds(body: string, status = 200) {
  // 204 and friends are defined as bodyless, so the constructor rejects even an empty string.
  const nullBodyStatus = [101, 103, 204, 205, 304].includes(status);
  return async () => new Response(nullBodyStatus ? null : body, { status });
}

function lastRequest() {
  const call = vi.mocked(fetch).mock.calls.at(-1);
  if (!call) throw new Error('fetch was never called');
  return { url: String(call[0]), init: call[1] as RequestInit };
}

function headerOf(name: string) {
  return new Headers(lastRequest().init.headers).get(name);
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(responds('{}')));
  vi.mocked(getAccessToken).mockResolvedValue(TOKEN);
  vi.mocked(clientAddress).mockResolvedValue(undefined);
  vi.mocked(getActiveBranchId).mockResolvedValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('apiRequest bodies', () => {
  it('parses a JSON body', async () => {
    vi.mocked(fetch).mockImplementation(responds('{"id":"abc"}'));
    await expect(apiRequest({ path: '/v1/me' })).resolves.toEqual({ id: 'abc' });
  });

  /*
   * Emptiness is read off the body, not off the status. The API answers 204 for a completed
   * write and 202 for an accepted one; treating only 204 as bodyless made forgot-password's
   * 202 render as an error.
   */
  it.each([202, 204])('treats a bodyless %i as success, not a parse failure', async (status) => {
    vi.mocked(fetch).mockImplementation(responds('', status));
    await expect(apiRequest({ path: '/v1/public/auth/forgot-password' })).resolves.toBeUndefined();
  });

  it('rejects a success whose body is not JSON', async () => {
    vi.mocked(fetch).mockImplementation(responds('<html>nope</html>'));
    await expect(apiRequest({ path: '/v1/me' })).rejects.toMatchObject({ code: 'unexpected' });
  });
});

describe('apiRequest error vocabulary', () => {
  it.each([
    [400, 'badRequest'],
    [401, 'unauthenticated'],
    [403, 'forbidden'],
    [404, 'notFound'],
    [409, 'conflict'],
    [422, 'unprocessable'],
    [429, 'rateLimited'],
    [500, 'unexpected'],
    [418, 'unexpected'],
  ])('maps %i onto %s', async (status, code) => {
    vi.mocked(fetch).mockImplementation(responds('{}', status));
    await expect(apiRequest({ path: '/v1/me' })).rejects.toMatchObject({ code, status });
  });

  // An unreachable API is not a rejected credential and must not read as one.
  it('maps a transport failure onto unreachable with no status', async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError('connect ECONNREFUSED'));
    await expect(apiRequest({ path: '/v1/me' })).rejects.toMatchObject({
      code: 'unreachable',
      status: 0,
    });
  });

  it('keeps the envelope for the log without promising it to a screen', async () => {
    vi.mocked(fetch).mockImplementation(
      responds('{"error":"conflict","detail":"email taken"}', 409),
    );
    await expect(apiRequest({ path: '/v1/users' })).rejects.toThrow('conflict: email taken');
  });

  it('survives an error body that is not the envelope', async () => {
    vi.mocked(fetch).mockImplementation(responds('gateway timeout', 504));
    await expect(apiRequest({ path: '/v1/me' })).rejects.toMatchObject({ code: 'unexpected' });
  });
});

describe('apiRequest headers', () => {
  it('sends the bearer on an authenticated call', async () => {
    await apiRequest({ path: '/v1/me' });
    expect(headerOf('Authorization')).toBe(`Bearer ${TOKEN}`);
  });

  // Middleware let a public route through, or the session died between the gate and here.
  // Either way it is the answer the API would give, without spending the round trip.
  it('refuses an authenticated call with no token, before fetching', async () => {
    vi.mocked(getAccessToken).mockResolvedValue(undefined);
    await expect(apiRequest({ path: '/v1/me' })).rejects.toMatchObject({
      code: 'unauthenticated',
      status: 401,
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it('attaches no bearer to a public call', async () => {
    await apiRequest({ path: '/v1/public/auth/login', authenticated: false });
    expect(headerOf('Authorization')).toBeNull();
    expect(getAccessToken).not.toHaveBeenCalled();
  });

  // Inheriting the caller's branch is what stops every screen re-deciding its own scope.
  it("names the caller's active branch without being asked", async () => {
    vi.mocked(getActiveBranchId).mockResolvedValue('branch-1');
    await apiRequest({ path: '/v1/products' });
    expect(headerOf('X-Branch-Id')).toBe('branch-1');
  });

  // A write prepared for one branch must land there even if the caller switched in between.
  it('lets an explicit branch pin the request over the active one', async () => {
    vi.mocked(getActiveBranchId).mockResolvedValue('branch-1');
    await apiRequest({ path: '/v1/product-prices/import/confirm', branchId: 'branch-2' });
    expect(headerOf('X-Branch-Id')).toBe('branch-2');
  });

  // Absent means account-wide for an admin and the assigned set for a seller, so an empty
  // value must not be sent as if it were a choice.
  it('omits the branch header when none is active', async () => {
    await apiRequest({ path: '/v1/products' });
    expect(headerOf('X-Branch-Id')).toBeNull();
  });

  /*
   * Identity and the branch list opt out, and they must: a stale cookie would 403 the caller's
   * own identity and sign them out, and 403 the very list they need to switch away from it.
   */
  it('sends no branch on a call that opted out, and does not even read one', async () => {
    vi.mocked(getActiveBranchId).mockResolvedValue('branch-1');
    await apiRequest({ path: '/v1/me', branchScoped: false });
    expect(headerOf('X-Branch-Id')).toBeNull();
    expect(getActiveBranchId).not.toHaveBeenCalled();
  });

  it('ignores an explicit branch on a call that opted out', async () => {
    await apiRequest({ path: '/v1/branches', branchScoped: false, branchId: 'branch-1' });
    expect(headerOf('X-Branch-Id')).toBeNull();
  });

  /*
   * Without this the API counts every user's requests against this server's address and one
   * allowance covers the whole product.
   */
  it("forwards the browser's address when the deployment declares a proxy", async () => {
    vi.mocked(clientAddress).mockResolvedValue('203.0.113.9');
    await apiRequest({ path: '/v1/me' });
    expect(headerOf('X-Forwarded-For')).toBe('203.0.113.9');
  });

  it('forwards no address when nothing sits in front', async () => {
    await apiRequest({ path: '/v1/me' });
    expect(headerOf('X-Forwarded-For')).toBeNull();
  });

  it('declares JSON only when it sends a body', async () => {
    await apiRequest({ path: '/v1/users', method: 'POST', body: { name: 'Ana' } });
    expect(headerOf('Content-Type')).toBe('application/json');
    expect(lastRequest().init.body).toBe('{"name":"Ana"}');

    await apiRequest({ path: '/v1/users' });
    expect(headerOf('Content-Type')).toBeNull();
  });
});

describe('apiRequest query strings', () => {
  it('appends nothing when there is no query', async () => {
    await apiRequest({ path: '/v1/products' });
    expect(lastRequest().url.endsWith('/v1/products')).toBe(true);
  });

  // An omitted filter and an empty one are different asks, so undefined is dropped rather
  // than sent as a blank value the API would try to match on.
  it('drops undefined entries and keeps empty strings', async () => {
    await apiRequest({ path: '/v1/products', query: { search: '', category: undefined } });
    expect(lastRequest().url).toContain('?search=');
    expect(lastRequest().url).not.toContain('category');
  });

  it('appends nothing when every entry is undefined', async () => {
    await apiRequest({ path: '/v1/products', query: { search: undefined } });
    expect(lastRequest().url.endsWith('/v1/products')).toBe(true);
  });

  it('encodes values rather than pasting them in', async () => {
    await apiRequest({ path: '/v1/products', query: { search: 'cal & arena' } });
    expect(lastRequest().url).toContain('search=cal+%26+arena');
  });
});

describe('errorCodeOf', () => {
  it('reads the code off an ApiError', () => {
    expect(errorCodeOf(new ApiError('conflict', 409))).toBe('conflict');
  });

  it.each([new Error('boom'), 'a string', null, undefined])(
    'calls anything else unexpected (%p)',
    (thrown) => {
      expect(errorCodeOf(thrown)).toBe('unexpected');
    },
  );
});
