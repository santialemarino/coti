import { NextResponse, type NextRequest } from 'next/server';

import { ROUTES } from '@/config/routes';
import { clearActiveBranch } from '@/lib/auth/branch';

// Where the protected layout sends a stale branch cookie, because a layout cannot write cookies.
export async function GET(request: NextRequest) {
  await clearActiveBranch();
  return NextResponse.redirect(new URL(ROUTES.home, request.url));
}
