import { afterEach, describe, expect, it, vi } from 'vitest';

import { listSellers } from '@/lib/api/sellers';

afterEach(() => vi.unstubAllGlobals());

function answer(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(body), { status })),
  );
}

describe('listSellers', () => {
  it('maps the picklist and names the branch it asks about', async () => {
    answer(200, { items: [{ id: 'u1', name: 'Ana Gómez' }] });

    await expect(listSellers('b1')).resolves.toEqual([{ id: 'u1', name: 'Ana Gómez' }]);
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe('/api/users?branch_id=b1');
  });

  // An empty list means a branch with no sellers, which a screen explains; a failure must not pass
  // for one.
  it('throws on a refused load instead of answering an empty branch', async () => {
    answer(500);

    await expect(listSellers('b1')).rejects.toThrow();
  });
});
