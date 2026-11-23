import 'server-only';

import type { ChannelType } from '@/lib/api/channels';
import { apiRequest } from '@/lib/api/client';

interface BranchChannelRaw {
  id: string;
  branch_id: string;
  type: ChannelType;
  identifier: string | null;
  is_active: boolean;
  is_configured: boolean;
}

export interface BranchChannel {
  id: string;
  branchId: string;
  type: ChannelType;
  identifier: string | null;
  isActive: boolean;
  isConfigured: boolean;
}

function mapChannel(raw: BranchChannelRaw): BranchChannel {
  return {
    id: raw.id,
    branchId: raw.branch_id,
    type: raw.type,
    identifier: raw.identifier,
    isActive: raw.is_active,
    isConfigured: raw.is_configured,
  };
}

export async function getBranchChannels(branchId: string): Promise<BranchChannel[]> {
  const { items } = await apiRequest<{ items: BranchChannelRaw[] }>({
    path: '/v1/channels',
    branchId,
    query: { include_inactive: 'true' },
  });
  return items.map(mapChannel);
}
