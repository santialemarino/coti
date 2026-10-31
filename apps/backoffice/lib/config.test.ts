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
