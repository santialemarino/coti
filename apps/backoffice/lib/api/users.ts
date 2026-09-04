import 'server-only';

import { cache } from 'react';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface UserListRaw {
  items: UserRaw[];
}

interface UserRaw {
  id: string;
  name: string;
  email: string;
  role: string;
  is_active: boolean;
  branch_ids: string[];
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
}

// --- Frontend types (camelCase) ---

export interface AccountUser {
  id: string;
  name: string;
  email: string;
  role: string;
  isActive: boolean;
  branchIds: string[];
  // Null until they log in for the first time, so a screen can tell "never" from "long ago".
  lastLoginAt: string | null;
}

// --- Mappers ---

function mapUser(raw: UserRaw): AccountUser {
  return {
    id: raw.id,
    name: raw.name,
    email: raw.email,
    role: raw.role,
    isActive: raw.is_active,
    branchIds: raw.branch_ids,
    lastLoginAt: raw.last_login_at,
  };
}

// --- API functions ---

/*
 * The account's users, ordered by name and including the deactivated ones so they can be
 * re-enabled. Admin-only on the API, which answers 403 to a seller.
 */
export const getUsers = cache(async (): Promise<AccountUser[]> => {
  const { items } = await apiRequest<UserListRaw>({ path: '/v1/users' });
  return items.map(mapUser);
});
