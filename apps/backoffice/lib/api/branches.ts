import 'server-only';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

// Collections come wrapped in an items envelope, so a list can grow pagination
// without breaking its callers.
interface BranchListRaw {
  items: BranchRaw[];
}

interface BranchRaw {
  id: string;
  name: string;
  address: string | null;
  default_expiry_days: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// --- Frontend types (camelCase) ---

export interface Branch {
  id: string;
  name: string;
  address: string | null;
  defaultExpiryDays: number;
  isActive: boolean;
}

// --- Mappers ---

function mapBranch(raw: BranchRaw): Branch {
  return {
    id: raw.id,
    name: raw.name,
    address: raw.address,
    defaultExpiryDays: raw.default_expiry_days,
    isActive: raw.is_active,
  };
}

// --- API functions ---

export async function getBranches(): Promise<Branch[]> {
  const { items } = await apiRequest<BranchListRaw>({ path: '/v1/branches' });
  return items.map(mapBranch);
}
