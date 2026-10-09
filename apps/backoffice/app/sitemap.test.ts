import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/utils/site-origin', () => ({ siteOrigin: vi.fn(async () => 'https://coti.test') }));

const { default: sitemap } = await import('@/app/sitemap');

describe('sitemap', () => {
  it('lists the public pages by their canonical addresses and nothing else', async () => {
    expect((await sitemap()).map((entry) => entry.url)).toEqual([
      'https://coti.test/',
      'https://coti.test/privacy',
      'https://coti.test/terms',
    ]);
  });
});
