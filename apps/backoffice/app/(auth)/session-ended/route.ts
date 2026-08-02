import { NextResponse, type NextRequest } from 'next/server';

import { LOGIN_ROUTE } from '@/config/routes';
import { clearSession } from '@/lib/auth/session';

/*
 * Where the protected layout sends a caller whose session the API has ended — a
 * bumped epoch, a deactivated user, a revoked token. It exists because a layout
 * cannot write cookies: redirecting straight to the login screen with the dead
 * cookies still set would have middleware bounce the caller back, forever.
 */
export async function GET(request: NextRequest) {
  await clearSession();
  return NextResponse.redirect(new URL(LOGIN_ROUTE, request.url));
}
