import { ROUTES } from '@/config/routes';

export const PORTAL_SLUG_HEADER = 'x-coti-portal-slug';
export const PORTAL_SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

export function portalSlug(value: string | null): string {
  return value && PORTAL_SLUG_PATTERN.test(value) ? value : '';
}

export function portalPath(path: string, slug: string): string {
  if (!slug || !path.startsWith('/') || path.startsWith('//')) return path;
  const prefix = ROUTES.portal(slug);
  if (path === prefix || path.startsWith(prefix + '/') || path.startsWith(prefix + '?'))
    return path;
  return prefix + (path === '/' ? '' : path);
}

export function googlePortalSlug(state: string): string {
  const match = /^[A-Za-z0-9_-]{43}\.p\.([a-z0-9-]+)$/.exec(state);
  return portalSlug(match?.[1] ?? null);
}

export function browserPortalPath(path: string): string {
  const pathname = typeof window === 'undefined' ? '' : window.location.pathname;
  const match = /^\/c\/([^/]+)(?:\/|$)/.exec(pathname);
  return portalPath(path, portalSlug(match?.[1] ?? null));
}
