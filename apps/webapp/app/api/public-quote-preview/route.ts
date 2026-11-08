import { NextRequest, NextResponse } from 'next/server';

import {
  previewPublicQuoteSelection,
  PublicQuoteSelectionPreviewError,
} from '@/lib/api/public-quotes';
import { getCustomerToken } from '@/lib/auth/customer-session';
import { isSameOrigin } from '@/lib/auth/same-origin';

/* The browser needs this proxy because customer API calls are server-side and token-authorized. */
export async function POST(request: NextRequest) {
  if (!isSameOrigin(request)) return NextResponse.json({ error: 'FORBIDDEN' }, { status: 403 });
  if (!(await getCustomerToken()))
    return NextResponse.json({ error: 'UNAUTHENTICATED' }, { status: 401 });
  const body = (await request.json().catch(() => null)) as {
    token?: unknown;
    selections?: unknown;
  } | null;
  if (
    !body ||
    typeof body.token !== 'string' ||
    body.token === '' ||
    !Array.isArray(body.selections)
  ) {
    return NextResponse.json({ error: 'BAD_REQUEST' }, { status: 400 });
  }
  const selections = body.selections.map((selection) => {
    if (
      !selection ||
      typeof selection !== 'object' ||
      typeof (selection as { item_id?: unknown }).item_id !== 'string' ||
      typeof (selection as { alternative_id?: unknown }).alternative_id !== 'string'
    ) {
      return null;
    }
    return {
      itemId: (selection as { item_id: string }).item_id,
      alternativeId: (selection as { alternative_id: string }).alternative_id,
    };
  });
  if (selections.some((selection) => selection === null)) {
    return NextResponse.json({ error: 'BAD_REQUEST' }, { status: 400 });
  }
  try {
    const result = await previewPublicQuoteSelection(
      body.token,
      selections.filter(
        (selection): selection is { itemId: string; alternativeId: string } => selection !== null,
      ),
    );
    return NextResponse.json(result);
  } catch (error) {
    if (error instanceof PublicQuoteSelectionPreviewError) {
      if (error.status === 401)
        return NextResponse.json({ error: 'UNAUTHENTICATED' }, { status: 401 });
      if (error.status === 404 || error.status === 409)
        return NextResponse.json({ error: 'CONFLICT' }, { status: error.status });
    }
    return NextResponse.json({ error: 'UNREACHABLE' }, { status: 502 });
  }
}
