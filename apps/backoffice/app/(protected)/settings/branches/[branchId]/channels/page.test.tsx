import { beforeEach, describe, expect, it, vi } from 'vitest';

import BranchChannelsPage from '@/app/(protected)/settings/branches/[branchId]/channels/page';
import { getBranchChannels } from '@/lib/api/branch-channels';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('next/navigation', () => ({
  notFound: () => {
    throw new Error('not found');
  },
}));
vi.mock('@/lib/api/branch-channels', () => ({ getBranchChannels: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));

const BRANCH = {
  id: 'b2',
  name: 'Moron',
  email: null,
  address: null,
  defaultExpiryDays: 7,
  isActive: true,
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getBranches).mockResolvedValue([BRANCH]);
  vi.mocked(getBranchChannels).mockResolvedValue([]);
});

describe('BranchChannelsPage', () => {
  it('requires an administrator and reads the branch named in the route', async () => {
    await BranchChannelsPage({ params: Promise.resolve({ branchId: BRANCH.id }) });
    expect(requireAdmin).toHaveBeenCalledOnce();
    expect(getBranchChannels).toHaveBeenCalledWith(BRANCH.id);
  });

  it('does not request channels for a branch outside the authenticated account', async () => {
    await expect(
      BranchChannelsPage({ params: Promise.resolve({ branchId: 'other-account-branch' }) }),
    ).rejects.toThrow('not found');
    expect(getBranchChannels).not.toHaveBeenCalled();
  });
});
