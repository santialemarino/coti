import { NextRequest } from 'next/server';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/auth/session', () => ({ clearSession: vi.fn() }));

const { clearSession } = await import('@/lib/auth/session');
const { GET } = await import('@/app/(auth)/session-ended/route');

// Relative, because a route handler's request.url names the server's own hostname, not the browser's.
describe('GET /session-ended', () => {
  it('clears the session and sends the caller to log in', async () => {
    const response = await GET(new NextRequest('https://backoffice.test/session-ended'));

    expect(clearSession).toHaveBeenCalledTimes(1);
    expect(response.headers.get('location')).toBe('/login');
  });

  it('carries the locked reason to the login screen', async () => {
    const response = await GET(
      new NextRequest('https://backoffice.test/session-ended?reason=locked'),
    );

    expect(response.headers.get('location')).toBe('/login?reason=locked');
  });

  it('forwards no reason it does not know', async () => {
    const response = await GET(
      new NextRequest('https://backoffice.test/session-ended?reason=%3Cscript%3E'),
    );

    expect(response.headers.get('location')).toBe('/login');
  });
});
