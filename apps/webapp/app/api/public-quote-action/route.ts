import { NextRequest, NextResponse } from 'next/server';

import {
  PublicQuoteActionError,
  submitPublicQuoteAction,
  type CustomerActionType,
} from '@/lib/api/public-quotes';
import { getCustomerToken } from '@/lib/auth/customer-session';
import { isSameOrigin } from '@/lib/auth/same-origin';

const ACTION_TYPES: ReadonlySet<string> = new Set(['ACCEPT', 'REQUEST_CHANGE', 'REJECT']);

/*
 * The browser cannot call the API (no CORS), so the customer's answer goes through this same-origin
 * route, which forwards it server-side. The page already validated everything the backend will
 * check again; this proxy only guards the boundary between the browser and the server module.
 */
export async function POST(request: NextRequest) {
  if (!isSameOrigin(request)) return NextResponse.json({ error: 'FORBIDDEN' }, { status: 403 });
  if (!(await getCustomerToken()))
    return NextResponse.json({ error: 'UNAUTHENTICATED' }, { status: 401 });
  const body = (await request.json().catch(() => null)) as {
    token?: unknown;
    type?: unknown;
    message?: unknown;
  } | null;
  if (
    !body ||
    typeof body.token !== 'string' ||
    body.token === '' ||
    typeof body.type !== 'string' ||
    !ACTION_TYPES.has(body.type)
  ) {
    return NextResponse.json({ error: 'BAD_REQUEST' }, { status: 400 });
  }
  const message =
    typeof body.message === 'string' && body.message.trim() !== ''
      ? body.message.trim()
      : undefined;
  try {
    const result = await submitPublicQuoteAction(body.token, {
      type: body.type as CustomerActionType,
      message,
    });
    return NextResponse.json(result);
  } catch (error) {
    if (error instanceof PublicQuoteActionError) {
      if (error.status === 401)
        return NextResponse.json({ error: 'UNAUTHENTICATED' }, { status: 401 });
      if (error.status === 404 || error.status === 409) {
        return NextResponse.json({ error: conflictCode(error) }, { status: error.status });
      }
      return NextResponse.json({ error: 'UNREACHABLE' }, { status: 502 });
    }
    return NextResponse.json({ error: 'UNREACHABLE' }, { status: 502 });
  }
}

// The one refusal the page words on its own: the corralón archived the quote, so the offer is gone.
function conflictCode(error: PublicQuoteActionError): string {
  if (error.status === 404) return 'NOT_FOUND';
  return error.code === 'QUOTE_ARCHIVED' ? 'QUOTE_ARCHIVED' : 'CONFLICT';
}
