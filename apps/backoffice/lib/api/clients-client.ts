import {
  mapClientMatch,
  mapClientTag,
  mapQuoteClientAssociation,
  type ClientMatch,
  type ClientMatchRaw,
  type ClientTag,
  type ClientTagListRaw,
  type ClientTagRaw,
  type QuoteClientAssociation,
  type QuoteClientAssociationRaw,
} from '@/lib/api/client-profiles';
import { ApiError, codeForStatus, knownErrorCode } from '@/lib/api/errors';

export interface AssociateQuoteClientBody {
  client_id?: string;
  new_client?: {
    name?: string;
    phone?: string;
    email?: string;
  };
  tag_ids: string[];
}

export async function getQuoteClientAssociation(
  quoteId: string,
  branchId: string,
): Promise<QuoteClientAssociation> {
  const response = await fetch(`/api/quotes/${quoteId}/client-association`, {
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  const raw = (await response.json()) as QuoteClientAssociationRaw;
  return mapQuoteClientAssociation(raw);
}

export async function associateQuoteClient(
  quoteId: string,
  branchId: string,
  body: AssociateQuoteClientBody,
): Promise<ClientMatch> {
  const response = await fetch(`/api/quotes/${quoteId}/client-association`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-Branch-Id': branchId },
    body: JSON.stringify(body),
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapClientMatch((await response.json()) as ClientMatchRaw);
}

export async function createClientTag(name: string): Promise<ClientTag> {
  const response = await fetch('/api/tags', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapClientTag((await response.json()) as ClientTagRaw);
}

export async function replaceClientTags(clientId: string, tagIds: string[]): Promise<ClientTag[]> {
  const response = await fetch(`/api/clients/${clientId}/tags`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tag_ids: tagIds }),
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  const raw = (await response.json()) as ClientTagListRaw;
  return raw.items.map(mapClientTag);
}

async function throwOnError(response: Response): Promise<never> {
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
    // A non-envelope body adds no information beyond the status.
  }
  throw new ApiError(
    knownErrorCode(code) ?? codeForStatus(response.status),
    response.status,
    detail || undefined,
  );
}
