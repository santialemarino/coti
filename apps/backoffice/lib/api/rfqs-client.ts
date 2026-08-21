import { ApiError, codeForStatus, knownErrorCode } from '@/lib/api/errors';

interface CreateRfqBody {
  client_label?: string | null;
  work_type?: string | null;
  items: {
    product_id?: string | null;
    requested_description: string;
    quantity: string;
    unit?: string | null;
  }[];
}

export interface CreateRfqResponse {
  rfq: {
    id: string;
    branch_id: string;
    client_label: string | null;
    channel_id: string;
    raw_text: string | null;
    status: string;
    work_type: string | null;
    received_at: string;
    created_at: string;
  };
  quote: {
    id: string;
    branch_id: string;
    rfq_id: string;
    seller_id: string | null;
    current_version_id: string | null;
    current_status: string;
    created_at: string;
    updated_at: string;
  };
}

/*
 * Client-side RFQ creation. Posts to the internal BFF route that reads the
 * session and branch cookies server-side and forwards them as auth headers.
 * This module has no server-only imports and is safe to use from Client Components.
 */
export async function createRfq(body: CreateRfqBody): Promise<CreateRfqResponse> {
  const response = await fetch('/api/rfqs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    let code: string | undefined;
    let detail = '';
    try {
      const payload = (await response.json()) as {
        error?: string;
        code?: string;
        detail?: string;
      };
      code = payload.code;
      detail = [payload.error, payload.detail].filter(Boolean).join(': ');
    } catch {
      // A body that is not the envelope tells us nothing extra.
    }
    throw new ApiError(
      knownErrorCode(code) ?? codeForStatus(response.status),
      response.status,
      detail || undefined,
    );
  }

  return response.json() as Promise<CreateRfqResponse>;
}
