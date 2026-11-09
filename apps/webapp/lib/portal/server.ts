import 'server-only';

import { headers } from 'next/headers';
import { redirect } from 'next/navigation';

import { PORTAL_SLUG_HEADER, portalPath, portalSlug } from '@/lib/portal/routing';

export async function customerPortalURL(hostname: string, path: string): Promise<string> {
  const host = (await headers()).get('host') || '';
  const name = host.toLowerCase().replace(/:\d+$/, '').replace(/\.$/, '');
  const suffix = '.' + name;
  const slug = hostname.endsWith(suffix) ? portalSlug(hostname.slice(0, -suffix.length)) : '';
  if (!slug) return 'https://' + hostname + path;
  const scheme = process.env.NODE_ENV === 'production' ? 'https' : 'http';
  return `${scheme}://${host}${portalPath(path, slug)}`;
}

export async function getPortalSlug(): Promise<string> {
  return portalSlug((await headers()).get(PORTAL_SLUG_HEADER));
}

export async function customerRedirect(path: string): Promise<never> {
  redirect(portalPath(path, await getPortalSlug()));
}
