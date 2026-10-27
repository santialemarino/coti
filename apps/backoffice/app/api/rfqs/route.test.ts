import { beforeEach, describe, expect, it, vi } from 'vitest';

import { GET } from '@/app/api/rfqs/route';

vi.mock('@/lib/api/upstream', () => ({ forwardToApi: vi.fn(async () => new Response('[]')) }));

const { forwardToApi } = await import('@/lib/api/upstream');

beforeEach(() => vi.mocked(forwardToApi).mockClear());

describe('GET /api/rfqs', () => {
  // The browser's re-read must scope as the server's read does, which never falls back to one branch.
  it('reads the open queue without falling back to the only branch', async () => {
    await GET(new Request('http://localhost/api/rfqs'));

    expect(vi.mocked(forwardToApi).mock.calls[0]?.[1]).toMatchObject({
      path: '/v1/rfqs',
      method: 'GET',
      fallbackToOnlyBranch: false,
    });
  });

  it('brings the archived orders only when the caller asks for them', async () => {
    await GET(new Request('http://localhost/api/rfqs?include_archived=true'));
    await GET(new Request('http://localhost/api/rfqs?include_archived=yes'));

    expect(vi.mocked(forwardToApi).mock.calls.map((call) => call[1].path)).toEqual([
      '/v1/rfqs?include_archived=true',
      '/v1/rfqs',
    ]);
  });
});
