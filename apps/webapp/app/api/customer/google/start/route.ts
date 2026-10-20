import { createHash, randomBytes } from 'node:crypto';
import { cookies } from 'next/headers';
import { NextRequest, NextResponse } from 'next/server';

import { ROUTES } from '@/config/routes';
import {
  CUSTOMER_COOKIE_OPTIONS,
  GOOGLE_STATE_COOKIE,
  safeCustomerDestination,
} from '@/lib/auth/customer-flow';
import { customerApiPath } from '@/lib/auth/customer-session';
import { GOOGLE_FLOW_TTL_SECONDS } from '@/lib/config';

export async function GET(request: NextRequest) {
  const response = await fetch(await customerApiPath('/auth/google/options'), {
    cache: 'no-store',
  });
  if (!response.ok)
    return NextResponse.redirect(new URL(`${ROUTES.login}?error=unavailable`, request.url));
  const options = (await response.json()) as { client_id: string; redirect_uri: string };
  if (!options.client_id)
    return NextResponse.redirect(new URL(`${ROUTES.login}?error=googleUnavailable`, request.url));
  const state = randomBytes(32).toString('base64url');
  const nonce = randomBytes(32).toString('base64url');
  const verifier = randomBytes(32).toString('base64url');
  const next = safeCustomerDestination(request.nextUrl.searchParams.get('next'));
  (await cookies()).set(
    GOOGLE_STATE_COOKIE,
    JSON.stringify({
      state,
      nonce,
      verifier,
      next,
      expiresAt: Date.now() + GOOGLE_FLOW_TTL_SECONDS * 1000,
    }),
    { ...CUSTOMER_COOKIE_OPTIONS, maxAge: GOOGLE_FLOW_TTL_SECONDS },
  );
  const url = new URL('https://accounts.google.com/o/oauth2/v2/auth');
  url.search = new URLSearchParams({
    client_id: options.client_id,
    redirect_uri: options.redirect_uri,
    response_type: 'code',
    scope: 'openid email profile',
    state,
    nonce,
    code_challenge: createHash('sha256').update(verifier).digest('base64url'),
    code_challenge_method: 'S256',
  }).toString();
  const result = NextResponse.redirect(url);
  result.headers.set('Cache-Control', 'no-store');
  return result;
}
