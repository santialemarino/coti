import { throwApiError } from '@/lib/api/errors';
import type { FileRfqDraftResponse } from '@/lib/api/rfqs';

export type NonOrderKind = 'GREETING' | 'QUESTION' | 'OTHER' | 'AUTOMATED';

// A channel message that opened no order: a greeting, a question or an automated mail.
export interface NonOrderMessage {
  id: string;
  kind: NonOrderKind;
  senderId: string;
  senderLabel: string | null;
  body: string;
  receivedAt: string;
}

interface NonOrderMessageRaw {
  id: string;
  kind: NonOrderKind;
  sender_id: string;
  sender_label: string | null;
  body: string;
  received_at: string;
}

export async function fetchNonOrders(branchId: string): Promise<NonOrderMessage[]> {
  const response = await fetch('/api/inbound-messages/non-orders', {
    method: 'GET',
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  const body = (await response.json()) as { messages: NonOrderMessageRaw[] | null };
  return (body.messages ?? []).map((raw) => ({
    id: raw.id,
    kind: raw.kind,
    senderId: raw.sender_id,
    senderLabel: raw.sender_label,
    body: raw.body,
    receivedAt: raw.received_at,
  }));
}

export async function dismissNonOrder(id: string, branchId: string): Promise<void> {
  const response = await fetch(`/api/inbound-messages/${encodeURIComponent(id)}/dismiss`, {
    method: 'POST',
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
}

// Opens the order a seller says the message is; its draft answer has the same shape as a file's.
export async function createOrderFromNonOrder(
  id: string,
  branchId: string,
): Promise<FileRfqDraftResponse> {
  const response = await fetch(`/api/inbound-messages/${encodeURIComponent(id)}/order`, {
    method: 'POST',
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return response.json() as Promise<FileRfqDraftResponse>;
}
