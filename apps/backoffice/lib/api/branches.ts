import 'server-only';

import { authenticatedFetch } from '@/lib/auth';

interface BranchRaw {
  id: string;
  name: string;
}

export interface Branch {
  id: string;
  name: string;
}

function mapBranch(raw: BranchRaw): Branch {
  return { id: raw.id, name: raw.name };
}

export async function getBranches(): Promise<Branch[]> {
  const response = await authenticatedFetch('/v1/branches');
  if (!response.ok) throw new Error(`branches request failed: ${response.status}`);
  const branches: BranchRaw[] = await response.json();
  return branches.map(mapBranch);
}
