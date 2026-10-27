import { describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('@/lib/utils/site-origin', () => ({ siteOrigin: vi.fn(async () => 'https://coti.test') }));
vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(
    async () => (key: string) =>
      key
        .split('.')
        .reduce<unknown>(
          (node, segment) => (node as Record<string, unknown>)?.[segment],
          messages.meta,
        ),
  ),
}));

const { generatePublicPageMetadata } = await import('@/lib/utils/page');

describe('generatePublicPageMetadata', () => {
  it('names the canonical address and lets the page be indexed', async () => {
    const metadata = await generatePublicPageMetadata('privacy', '/privacy');

    expect(metadata.alternates?.canonical).toBe('https://coti.test/privacy');
    expect(metadata.robots).toEqual({ index: true, follow: true });
  });

  // Next replaces `openGraph` and `twitter` wholesale, so a card missing from either is a card lost.
  it('carries the social card in both the Open Graph and the Twitter tags', async () => {
    const metadata = await generatePublicPageMetadata('landing', '/');
    const card = 'https://coti.test/opengraph-image';

    expect(metadata.openGraph?.images).toEqual([expect.objectContaining({ url: card })]);
    expect(metadata.twitter?.images).toEqual([expect.objectContaining({ url: card })]);
    expect(metadata.openGraph?.url).toBe('https://coti.test/');
  });
});
