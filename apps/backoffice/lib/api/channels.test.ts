import { afterEach, describe, expect, it, vi } from 'vitest';

import { listChannels } from '@/lib/api/channels';

afterEach(() => vi.unstubAllGlobals());

function answer(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(body), { status })),
  );
}

describe('listChannels', () => {
  it('keeps only the open channels', async () => {
    answer(200, {
      items: [
        { id: 'c1', type: 'WHATSAPP', identifier: '+5491122542609', is_active: true },
        { id: 'c2', type: 'EMAIL', identifier: 'moron@corralon.test', is_active: false },
      ],
    });

    await expect(listChannels('b1')).resolves.toEqual([
      { id: 'c1', type: 'WHATSAPP', identifier: '+5491122542609' },
    ]);
  });

  // An empty list is a branch with no open channel, which a screen explains; a failure must not
  // pass for one.
  it('throws on a refused load instead of answering an empty branch', async () => {
    answer(500);

    await expect(listChannels('b1')).rejects.toThrow();
  });
});
