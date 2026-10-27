import 'server-only';

import { headers } from 'next/headers';

/*
 * The origin the request arrived on, which canonical URLs, the sitemap and the social card must name
 * absolutely. Read per request so one build serves whatever domain the deployment is given.
 */
export async function siteOrigin(): Promise<string> {
  const requestHeaders = await headers();
  const host = requestHeaders.get('x-forwarded-host') ?? requestHeaders.get('host') ?? '';
  const protocol = requestHeaders.get('x-forwarded-proto') ?? 'http';
  return `${protocol.split(',')[0]}://${host.split(',')[0]}`;
}
