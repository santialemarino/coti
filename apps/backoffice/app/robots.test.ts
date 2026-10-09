import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/utils/site-origin', () => ({ siteOrigin: vi.fn(async () => 'https://coti.test') }));

const { default: robots } = await import('@/app/robots');

describe('robots', () => {
  // The app behind the login and every order in it must never be crawled, only the public site.
  it('allows the public site and its card, and disallows everything else', async () => {
    const { rules, sitemap } = await robots();

    expect(rules).toEqual({
      userAgent: '*',
      allow: ['/$', '/privacy', '/terms', '/opengraph-image'],
      disallow: '/',
    });
    expect(sitemap).toBe('https://coti.test/sitemap.xml');
  });
});
