import 'server-only';

import { apiRequest } from '@/lib/api/client';
import {
  mapClientProfile,
  mapClientSummary,
  mapClientTag,
  type ClientListRaw,
  type ClientProfile,
  type ClientProfileRaw,
  type ClientSummary,
  type ClientTag,
  type ClientTagListRaw,
} from '@/lib/api/client-profiles';

export async function getClients(): Promise<ClientSummary[]> {
  const raw = await apiRequest<ClientListRaw>({ path: '/v1/clients' });
  return raw.items.map(mapClientSummary);
}

export async function getClient(clientId: string): Promise<ClientProfile> {
  const raw = await apiRequest<ClientProfileRaw>({ path: `/v1/clients/${clientId}` });
  return mapClientProfile(raw);
}

export async function getClientTags(): Promise<ClientTag[]> {
  const raw = await apiRequest<ClientTagListRaw>({ path: '/v1/tags', branchScoped: false });
  return raw.items.map(mapClientTag);
}
