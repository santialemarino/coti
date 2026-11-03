import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));

const { cookies } = await import('next/headers');
const { GET } = await import('@/app/api/reports/pdf/route');

const BRANCH_ID = '11111111-1111-4111-8111-111111111111';

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(cookies).mockResolvedValue(
    cookieJar({
      [ACCESS_COOKIE]: 'access-token',
      [BRANCH_COOKIE]: BRANCH_ID,
    }) as unknown as Awaited<ReturnType<typeof cookies>>,
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('the seller report PDF proxy', () => {
  it('forwards the period and the active branch', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('%PDF-1.4', {
        status: 200,
        headers: {
          'Content-Type': 'application/pdf',
          'Content-Disposition': 'attachment; filename="reporte.pdf"',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const response = await GET(
      new Request('http://localhost/api/reports/pdf?from=2026-10-01&to=2026-10-02'),
    );

    expect(response.status).toBe(200);
    expect(response.headers.get('Content-Type')).toBe('application/pdf');
    expect(response.headers.get('Content-Disposition')).toBe('attachment; filename="reporte.pdf"');
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe('http://localhost:8000/v1/reports/pdf?from=2026-10-01&to=2026-10-02');
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe(BRANCH_ID);
  });

  it('forwards every allowlisted filter, including repeated statuses', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('%PDF-1.4', {
        status: 200,
        headers: { 'Content-Type': 'application/pdf' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await GET(
      new Request(
        'http://localhost/api/reports/pdf?from=2026-10-01&to=2026-10-02&status=SENT&status=ACCEPTED' +
          '&channel=EMAIL&seller_id=seller-1&client_id=client-1&ignored=x',
      ),
    );

    const [url] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe(
      'http://localhost:8000/v1/reports/pdf?from=2026-10-01&to=2026-10-02&status=SENT&status=ACCEPTED' +
        '&channel=EMAIL&seller_id=seller-1&client_id=client-1',
    );
  });

  it('does not invent a branch when every branch is in view', async () => {
    vi.mocked(cookies).mockResolvedValue(
      cookieJar({ [ACCESS_COOKIE]: 'access-token' }) as unknown as Awaited<
        ReturnType<typeof cookies>
      >,
    );
    const genericFetch = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('%PDF-1.4', {
        status: 200,
        headers: { 'Content-Type': 'application/pdf' },
      }),
    );
    vi.stubGlobal('fetch', genericFetch);

    await GET(new Request('http://localhost/api/reports/pdf'));

    expect(genericFetch).toHaveBeenCalledTimes(1);
    const [, init] = genericFetch.mock.calls[0] ?? [];
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBeNull();
  });

  it('answers unauthenticated without reaching the API', async () => {
    vi.mocked(cookies).mockResolvedValue(
      cookieJar({}) as unknown as Awaited<ReturnType<typeof cookies>>,
    );
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal('fetch', fetchMock);

    const response = await GET(new Request('http://localhost/api/reports/pdf'));

    expect(response.status).toBe(401);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
