import 'server-only';

import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

export const BRANCH_HEADER = 'X-Branch-Id';

interface ForwardInit {
  path: string;
  method: string;
  body?: string;
  headers?: Record<string, string>;
  // Off for a read that must scope exactly as the server-side read does, which never falls back.
  fallbackToOnlyBranch?: boolean;
}

/*
 * Forwards one BFF route to the API with the caller's credentials. The route handlers do no
 * more than name the upstream path: the session cookie, the active branch and the error
 * envelope are the same on every one of them.
 */
export async function forwardToApi(
  incoming: Request | null,
  init: ForwardInit,
): Promise<NextResponse> {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${token}`);
  if (init.body !== undefined) {
    headers.set('Content-Type', 'application/json');
  }

  const branchId = await resolveBranchId(
    incoming,
    jar.get(BRANCH_COOKIE)?.value,
    token,
    init.fallbackToOnlyBranch ?? true,
  );
  if (branchId) {
    headers.set(BRANCH_HEADER, branchId);
  }

  const upstream = await fetch(`${API_URL}${init.path}`, {
    method: init.method,
    headers,
    body: init.body,
    cache: 'no-store',
  });

  if (upstream.ok && upstream.headers.get('Content-Type')?.startsWith('application/pdf')) {
    return new NextResponse(await upstream.arrayBuffer(), {
      status: upstream.status,
      headers: {
        'Content-Type': 'application/pdf',
        'Content-Disposition': upstream.headers.get('Content-Disposition') ?? 'attachment',
        'Cache-Control': 'private, no-store',
      },
    });
  }
  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}

/*
 * A branch-scoped write needs a branch even when the header switcher sits on "todas las
 * sucursales", so the screen that knows which branch owns the record names it. The API
 * validates whichever id arrives and answers 403 for one this caller may not use, so a named
 * branch is no more trusted than the cookie. Falling back to the only branch an account has
 * keeps a single-branch corralón from ever having to choose.
 */
async function resolveBranchId(
  incoming: Request | null,
  cookieBranchId: string | undefined,
  token: string,
  fallbackToOnlyBranch: boolean,
): Promise<string | undefined> {
  const named = incoming?.headers.get(BRANCH_HEADER) || undefined;
  if (named) return named;
  if (cookieBranchId) return cookieBranchId;
  if (!fallbackToOnlyBranch) return undefined;

  const response = await fetch(`${API_URL}/v1/branches`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
  });
  if (!response.ok) return undefined;

  const data = (await response.json()) as { items: { id: string }[] };
  return data.items.length === 1 ? data.items[0]?.id : undefined;
}
