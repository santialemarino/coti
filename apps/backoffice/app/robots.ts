import type { MetadataRoute } from 'next';

import { ROUTES } from '@/config/routes';
import { siteOrigin } from '@/lib/utils/site-origin';

// Only the public site and its social card are crawled; `$` keeps the rule for `/` from allowing
// every path under it.
export default async function robots(): Promise<MetadataRoute.Robots> {
  const origin = await siteOrigin();

  return {
    rules: {
      userAgent: '*',
      allow: [`${ROUTES.landing}$`, ROUTES.privacy, ROUTES.terms, '/opengraph-image'],
      disallow: '/',
    },
    sitemap: `${origin}/sitemap.xml`,
  };
}
