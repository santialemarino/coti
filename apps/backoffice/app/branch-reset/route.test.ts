import { NextRequest } from 'next/server';
import { describe, expect, it } from 'vitest';

import { GET } from '@/app/branch-reset/route';
import { ROUTES } from '@/config/routes';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';

function expectBranchCookieCleared(response: Response) {
  const cleared = response.headers.get('set-cookie') ?? '';
  expect(cleared).toContain(`${BRANCH_COOKIE}=;`);
  expect(cleared).toContain('Path=/');
  expect(cleared).toMatch(/Expires=Thu, 01 Jan 1970/);
}

describe('GET /branch-reset', () => {
  it('drops the branch cookie and sends the caller home', async () => {
    const response = await GET(new NextRequest('https://backoffice.test/branch-reset'));

    expect(response.status).toBe(307);
    expect(response.headers.get('location')).toBe(ROUTES.home);
    expectBranchCookieCleared(response);
  });

  // Followed home as flight data, the shell the router keeps would still name the dropped branch.
  it('answers a client navigation with no flight data, so the page loads in full', async () => {
    const response = await GET(
      new NextRequest('https://backoffice.test/branch-reset', { headers: { rsc: '1' } }),
    );

    expect(response.status).toBe(204);
    expect(response.headers.get('location')).toBeNull();
    expectBranchCookieCleared(response);
  });
});
