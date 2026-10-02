import type { NextRequest } from 'next/server';

import { ROUTES } from '@/config/routes';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';
import { redirectDocumentTo } from '@/lib/utils/redirect';

// Where a stale branch cookie is dropped, because neither a layout nor a page can write cookies.
export async function GET(request: NextRequest) {
  const response = redirectDocumentTo(request, ROUTES.home);
  response.cookies.delete(BRANCH_COOKIE);
  return response;
}
