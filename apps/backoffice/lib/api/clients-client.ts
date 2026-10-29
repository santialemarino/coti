import {
  clientMatchFromSummary,
  mapClient,
  mapClientSummary,
  mapClientTag,
  mapQuoteClientAssociation,
  type Client,
  type ClientListRaw,
  type ClientMatch,
  type ClientRaw,
  type ClientTag,
  type ClientTagListRaw,
  type ClientTagRaw,
  type QuoteClientAssociation,
  type QuoteClientAssociationRaw,
} from '@/lib/api/client-profiles';
import { throwApiError } from '@/lib/api/errors';
import {
  mapClientFiscal,
  type ClientFiscal,
  type ClientFiscalRaw,
  type IvaCondition,
} from '@/lib/api/invoicing';

export async function getClientDirectory(branchId: string): Promise<ClientMatch[]> {
  const response = await fetch('/api/clients', {
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  const raw = (await response.json()) as ClientListRaw;
  return raw.items.map(mapClientSummary).map(clientMatchFromSummary);
}

export async function getQuoteClientAssociation(
  quoteId: string,
  branchId: string,
): Promise<QuoteClientAssociation> {
  const response = await fetch(`/api/quotes/${quoteId}/client-association`, {
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  const raw = (await response.json()) as QuoteClientAssociationRaw;
  return mapQuoteClientAssociation(raw);
}

export async function createClientTag(name: string): Promise<ClientTag> {
  const response = await fetch('/api/tags', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapClientTag((await response.json()) as ClientTagRaw);
}

export async function replaceClientTags(clientId: string, tagIds: string[]): Promise<ClientTag[]> {
  const response = await fetch(`/api/clients/${clientId}/tags`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tag_ids: tagIds }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  const raw = (await response.json()) as ClientTagListRaw;
  return raw.items.map(mapClientTag);
}

export interface ClientFiscalInput {
  address?: string;
  legalName?: string;
  // A consumidor final's DNI. A CUIT is never sent: only the customer loads one, with their account.
  dni?: string;
  ivaCondition?: IvaCondition;
}

export async function updateClientFiscal(
  clientId: string,
  values: ClientFiscalInput,
): Promise<ClientFiscal> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/fiscal`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      address: values.address,
      legal_name: values.legalName,
      dni: values.dni,
      iva_condition: values.ivaCondition,
    }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapClientFiscal((await response.json()) as ClientFiscalRaw);
}

export async function renameClient(clientId: string, name: string): Promise<Client> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/name`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapClient((await response.json()) as ClientRaw);
}

export async function getMergeCandidates(clientId: string): Promise<Client[]> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/merge-candidates`, {
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  const raw = (await response.json()) as { items: ClientRaw[] };
  return raw.items.map(mapClient);
}

// Merges sourceId into clientId, which survives with both profiles' orders and contacts.
export async function mergeClient(clientId: string, sourceId: string): Promise<void> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source_client_id: sourceId }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
}
