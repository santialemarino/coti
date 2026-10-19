import 'server-only';

import { cache } from 'react';

import { API_URL } from '@/lib/config';

interface PublicRfqDestinationRaw {
  supplier_name: string;
  branch_name: string;
  logo_url?: string;
  max_text_characters: number;
  max_file_size: number;
  accepted_content_types: string[];
}

export interface PublicRfqDestination {
  supplierName: string;
  branchName: string;
  logoUrl?: string;
  maxTextCharacters: number;
  maxFileSize: number;
  acceptedContentTypes: string[];
}

function mapDestination(raw: PublicRfqDestinationRaw): PublicRfqDestination {
  return {
    supplierName: raw.supplier_name,
    branchName: raw.branch_name,
    logoUrl: raw.logo_url?.startsWith('/') ? `${API_URL}${raw.logo_url}` : raw.logo_url,
    maxTextCharacters: raw.max_text_characters,
    maxFileSize: raw.max_file_size,
    acceptedContentTypes: raw.accepted_content_types,
  };
}

export const getPublicRfqDestination = cache(
  async (channelId: string): Promise<PublicRfqDestination | null> => {
    if (!/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(channelId))
      return null;
    const response = await fetch(`${API_URL}/v1/public/rfq-channels/${channelId}`, {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    });
    if (response.status === 404) return null;
    if (!response.ok) throw new Error('Failed to resolve public RFQ destination');
    return mapDestination((await response.json()) as PublicRfqDestinationRaw);
  },
);

export interface PublicRfqSite {
  supplierName: string;
  branches: { channelId: string; destination: PublicRfqDestination }[];
}

export async function getPublicRfqSite(hostname: string): Promise<PublicRfqSite | null> {
  const response = await fetch(`${API_URL}/v1/public/rfq-sites/${encodeURIComponent(hostname)}`, {
    cache: 'no-store',
  });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error('Failed to resolve public RFQ site');
  const raw = (await response.json()) as Omit<PublicRfqDestinationRaw, 'branch_name'> & {
    branches: { channel_id: string; name: string }[];
  };
  return {
    supplierName: raw.supplier_name,
    branches: raw.branches.map((branch) => ({
      channelId: branch.channel_id,
      destination: mapDestination({ ...raw, branch_name: branch.name }),
    })),
  };
}
