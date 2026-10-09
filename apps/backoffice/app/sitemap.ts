import type { MetadataRoute } from 'next';

import { ROUTES } from '@/config/routes';
import { siteOrigin } from '@/lib/utils/site-origin';

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const origin = await siteOrigin();

  return [ROUTES.home, ROUTES.privacy, ROUTES.terms].map((path) => ({
    url: new URL(path, origin).toString(),
  }));
}
