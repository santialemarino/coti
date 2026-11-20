import 'server-only';

import { headers } from 'next/headers';
import { redirect } from 'next/navigation';

import { PORTAL_BASE_HOSTNAME, PORTAL_BASE_URL } from '@/lib/config';
import { PORTAL_SLUG_HEADER, portalPath, portalSlug } from '@/lib/portal/routing';

// The supplier's portal URL for path, or null when its hostname is not a `<label>.<base>` this
// portal serves (a legacy bare-base or custom hostname until `portal-backfill` converts it).
export function customerPortalURL(hostname: string, path: string): string | null {
  const suffix = '.' + PORTAL_BASE_HOSTNAME;
  const slug = hostname.endsWith(suffix) ? portalSlug(hostname.slice(0, -suffix.length)) : '';
  return slug ? PORTAL_BASE_URL + portalPath(path, slug) : null;
}

export async function getPortalSlug(): Promise<string> {
  return portalSlug((await headers()).get(PORTAL_SLUG_HEADER));
}

export async function customerRedirect(path: string): Promise<never> {
  redirect(portalPath(path, await getPortalSlug()));
}
