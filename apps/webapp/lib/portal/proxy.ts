import { NextRequest, NextResponse } from 'next/server';

import { LANDING_URL } from '@/lib/config';
import { googlePortalSlug, PORTAL_SLUG_HEADER, portalSlug } from '@/lib/portal/routing';

// Outside /c/<label> only these survive: the portal's own API routes (the shared Google callback
// among them) and the delivery links already sent, which resolve their supplier by token.
const UNSCOPED_PATHS = [/^\/api\//, /^\/quotes\/[^/]+\/?$/];

export function proxy(request: NextRequest) {
  const headers = new Headers(request.headers);
  headers.delete(PORTAL_SLUG_HEADER);
  const match = /^\/c\/([^/]+)(\/.*)?$/.exec(request.nextUrl.pathname);
  let slug = '';
  const url = request.nextUrl.clone();
  if (match) {
    slug = portalSlug(match[1] ?? null);
    if (!slug) return new NextResponse(null, { status: 404 });
    url.pathname = match[2] || '/';
    if (url.pathname.startsWith('/c/')) return new NextResponse(null, { status: 404 });
  } else if (!UNSCOPED_PATHS.some((pattern) => pattern.test(url.pathname))) {
    return NextResponse.redirect(LANDING_URL, 307);
  } else if (url.pathname === '/api/customer/google/callback') {
    slug = googlePortalSlug(url.searchParams.get('state') || '');
  }
  if (slug) headers.set(PORTAL_SLUG_HEADER, slug);
  return match
    ? NextResponse.rewrite(url, { request: { headers } })
    : NextResponse.next({ request: { headers } });
}
