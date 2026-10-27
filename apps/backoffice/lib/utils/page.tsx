import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { siteOrigin } from '@/lib/utils/site-origin';

// The card `app/opengraph-image.tsx` renders.
const SOCIAL_IMAGE = { path: '/opengraph-image', width: 1200, height: 630 } as const;

// Reads the meta namespace so a tab title is copy like any other, not a literal
// sitting in a .tsx file.
export async function generatePageMetadata(key: string): Promise<Metadata> {
  const t = await getTranslations('meta');
  return { title: `${t(`${key}.title`)} — Coti`, description: t(`${key}.description`) };
}

/*
 * A public page: indexable, with its canonical URL and the social card. Next replaces `openGraph`
 * and `twitter` wholesale instead of merging them, so every page restates both in full — leaving
 * either partial silently drops the image.
 */
export async function generatePublicPageMetadata(key: string, path: string): Promise<Metadata> {
  const t = await getTranslations('meta');
  const origin = await siteOrigin();
  const title = `${t(`${key}.title`)} — Coti`;
  const description = t(`${key}.description`);
  const url = new URL(path, origin).toString();
  const image = {
    url: new URL(SOCIAL_IMAGE.path, origin).toString(),
    width: SOCIAL_IMAGE.width,
    height: SOCIAL_IMAGE.height,
    alt: t('socialImageAlt'),
  };

  return {
    title,
    description,
    alternates: { canonical: url },
    robots: { index: true, follow: true },
    openGraph: {
      type: 'website',
      locale: 'es_AR',
      siteName: 'Coti',
      url,
      title,
      description,
      images: [image],
    },
    twitter: { card: 'summary_large_image', title, description, images: [image] },
  };
}
