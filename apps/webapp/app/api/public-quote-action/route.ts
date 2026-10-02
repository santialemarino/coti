import { NextRequest, NextResponse } from 'next/server';

import {
  PublicQuoteActionError,
  submitPublicQuoteAction,
  type CustomerActionType,
} from '@/lib/api/public-quotes';

const ACTION_TYPES: ReadonlySet<string> = new Set(['ACCEPT', 'REQUEST_CHANGE', 'REJECT']);

/*
 * The browser cannot call the API (no CORS), so the customer's answer goes through this same-origin
 * route, which forwards it server-side. The page already validated everything the backend will
 * check again; this proxy only guards the boundary between the browser and the server module.
 */
export async function POST(request: NextRequest) {
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
      if (error.status === 404 || error.status === 409) {
        return NextResponse.json(
          { error: error.status === 404 ? 'NOT_FOUND' : 'CONFLICT' },
          { status: error.status },
        );
      }
      return NextResponse.json({ error: 'UNREACHABLE' }, { status: 502 });
    }
    return NextResponse.json({ error: 'UNREACHABLE' }, { status: 502 });
  }
}
