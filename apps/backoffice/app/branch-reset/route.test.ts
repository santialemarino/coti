import { NextRequest } from 'next/server';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/auth/branch', () => ({ clearActiveBranch: vi.fn() }));

const { clearActiveBranch } = await import('@/lib/auth/branch');
const { GET } = await import('@/app/branch-reset/route');

describe('GET /branch-reset', () => {
  it('drops the branch cookie and sends the caller home', async () => {
    const response = await GET(new NextRequest('https://backoffice.test/branch-reset'));

    expect(clearActiveBranch).toHaveBeenCalledTimes(1);
    expect(response.headers.get('location')).toBe('https://backoffice.test/');
  });
});
