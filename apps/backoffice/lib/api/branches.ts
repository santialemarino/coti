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
}

// --- Frontend types (camelCase) ---

export interface Branch {
  id: string;
  name: string;
}

// --- Mappers ---

function mapBranch(raw: BranchRaw): Branch {
  return { id: raw.id, name: raw.name };
}

// --- API functions ---

export async function getBranches(): Promise<Branch[]> {
  const { items } = await apiRequest<BranchListRaw>({ path: '/v1/branches' });
  return items.map(mapBranch);
}
