import { NextRequest, NextResponse } from 'next/server';

import { googlePortalSlug, PORTAL_SLUG_HEADER, portalSlug } from '@/lib/portal/routing';

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
  } else if (url.pathname === '/api/customer/google/callback') {
    slug = googlePortalSlug(url.searchParams.get('state') || '');
  }
  if (slug) headers.set(PORTAL_SLUG_HEADER, slug);
  return match
    ? NextResponse.rewrite(url, { request: { headers } })
    : NextResponse.next({ request: { headers } });
}
