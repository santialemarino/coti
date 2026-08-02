import 'server-only';

import { cookies } from 'next/headers';

const ACCESS_COOKIE = 'coti_access_token';
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8000';

export async function authenticatedFetch(
  path: string,
  init: RequestInit = {},
  branchId?: string,
): Promise<Response> {
  const token = (await cookies()).get(ACCESS_COOKIE)?.value;
  if (!token) return new Response(null, { status: 401 });

  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${token}`);
  if (branchId) headers.set('X-Branch-Id', branchId);
  return fetch(`${API_URL}${path}`, { ...init, headers, cache: 'no-store' });
}
