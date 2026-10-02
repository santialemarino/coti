import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/quotes/:quoteId/sends. The API freezes the approved version and
 * attempts each destination independently, so the idempotency key has to survive a retry of
 * the same send: the browser mints it once per attempt and it is forwarded as-is.
 */
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  const idempotencyKey = request.headers.get('Idempotency-Key');
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/sends`,
    method: 'POST',
    body: await request.text(),
    headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
  });
}
