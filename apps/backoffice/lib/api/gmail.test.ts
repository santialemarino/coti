import { afterEach, describe, expect, it, vi } from 'vitest';

import { startGmailAuthorization } from '@/lib/api/gmail';

afterEach(() => vi.unstubAllGlobals());

describe('startGmailAuthorization', () => {
  it('selects the matching branch email channel and forwards the branch on authorization', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [
              { id: 'other', type: 'EMAIL', identifier: 'other@example.com', is_active: true },
              { id: 'correct', type: 'EMAIL', identifier: 'sales@example.com', is_active: true },
            ],
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            authorization_url: 'https://accounts.google.com/o/oauth2/v2/auth?state=abc',
          }),
          { status: 200 },
        ),
      );
    vi.stubGlobal('fetch', fetcher);
    const result = await startGmailAuthorization('branch-1', 'Sales@example.com');
    expect(result).toContain('https://accounts.google.com/');
    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/channels/correct/gmail/authorization', {
      method: 'POST',
      headers: { 'X-Branch-Id': 'branch-1' },
      cache: 'no-store',
    });
  });

  it('refuses a redirect outside Google', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            items: [
              { id: 'correct', type: 'EMAIL', identifier: 'sales@example.com', is_active: true },
            ],
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ authorization_url: 'https://accounts.google.com.attacker.test/' }),
          { status: 200 },
        ),
      );
    vi.stubGlobal('fetch', fetcher);
    await expect(startGmailAuthorization('branch-1', 'sales@example.com')).rejects.toThrow(
      'Invalid Google authorization URL',
    );
  });
});
