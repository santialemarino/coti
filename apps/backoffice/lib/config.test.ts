import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
});

describe('API URLs', () => {
  it('uses the internal URL for server requests without changing the public URL', async () => {
    vi.stubEnv('NEXT_PUBLIC_API_URL', 'https://app.coti.test/api');
    vi.stubEnv('API_INTERNAL_URL', 'http://api:8000');

    const { API_URL, PUBLIC_API_URL } = await import('@/lib/config');

    expect(API_URL).toBe('http://api:8000');
    expect(PUBLIC_API_URL).toBe('https://app.coti.test/api');
  });
});

describe('SIGNUP_ENABLED', () => {
  it.each([
    [undefined, true],
    ['', true],
    ['true', true],
    [' FALSE ', false],
    ['false', false],
  ])('reads %j as %s', async (raw, expected) => {
    if (raw !== undefined) vi.stubEnv('SIGNUP_ENABLED', raw);

    const { SIGNUP_ENABLED } = await import('@/lib/config');

    expect(SIGNUP_ENABLED).toBe(expected);
  });
});
