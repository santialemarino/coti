import { timingSafeEqual } from 'node:crypto';
import { cookies } from 'next/headers';
import { NextRequest, NextResponse } from 'next/server';

import { ROUTES } from '@/config/routes';
import {
  clearCustomerCookie,
  CUSTOMER_COOKIE_OPTIONS,
  GOOGLE_EMAIL_COOKIE,
  GOOGLE_SIGNUP_COOKIE,
  GOOGLE_STATE_COOKIE,
  postCustomerAuth,
  safeCustomerDestination,
} from '@/lib/auth/customer-flow';
import { acceptCustomerSession, type CustomerLoginRaw } from '@/lib/auth/customer-session';
import { GOOGLE_FLOW_TTL_SECONDS } from '@/lib/config';

export async function GET(request: NextRequest) {
  const jar = await cookies();
  const stored = jar.get(GOOGLE_STATE_COOKIE)?.value;
  jar.delete(GOOGLE_STATE_COOKIE);
  const fail = () => {
    const response = NextResponse.redirect(
      new URL(`${ROUTES.login}?error=googleFailed`, request.url),
    );
    response.headers.set('Cache-Control', 'no-store');
    return response;
  };
  try {
    const flow = JSON.parse(stored || '{}') as {
      state: string;
      verifier: string;
      nonce: string;
      next: string;
      expiresAt: number;
    };
    const state = request.nextUrl.searchParams.get('state') || '';
    const code = request.nextUrl.searchParams.get('code');
    if (
      !code ||
      !flow.state ||
      flow.expiresAt <= Date.now() ||
      !Number.isFinite(flow.expiresAt) ||
      state.length !== flow.state.length ||
      !timingSafeEqual(Buffer.from(state), Buffer.from(flow.state))
    )
      return fail();
    const response = await postCustomerAuth('/auth/google/exchange', {
      code,
      verifier: flow.verifier,
      nonce: flow.nonce,
    });
    if (!response.ok) return fail();
    const result = (await response.json()) as {
      session?: CustomerLoginRaw;
      signup_token?: string;
      signup_email?: string;
    };
    let destination = safeCustomerDestination(flow.next);
    if (result.session) await acceptCustomerSession(result.session);
    else if (result.signup_token && /^[A-Za-z0-9_-]{43}$/.test(result.signup_token)) {
      jar.set(GOOGLE_SIGNUP_COOKIE, result.signup_token, {
        ...CUSTOMER_COOKIE_OPTIONS,
        maxAge: GOOGLE_FLOW_TTL_SECONDS,
      });
      if (result.signup_email && result.signup_email.length <= 255)
        jar.set(GOOGLE_EMAIL_COOKIE, result.signup_email, {
          ...CUSTOMER_COOKIE_OPTIONS,
          maxAge: GOOGLE_FLOW_TTL_SECONDS,
        });
      else clearCustomerCookie(jar, GOOGLE_EMAIL_COOKIE);
      destination = `${ROUTES.googleComplete}?next=${encodeURIComponent(destination === ROUTES.orders ? ROUTES.newRequest : destination)}`;
    } else return fail();
    const redirect = NextResponse.redirect(new URL(destination, request.url));
    redirect.headers.set('Cache-Control', 'no-store');
    return redirect;
  } catch {
    return fail();
  }
}
