import type { MetadataRoute } from 'next';

// Every page here is a customer's quote reached through a private link; none is for a crawler.
export default function robots(): MetadataRoute.Robots {
  return { rules: { userAgent: '*', disallow: '/' } };
}
