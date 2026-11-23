import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getBranchChannels } from '@/lib/api/branch-channels';
import { apiRequest } from '@/lib/api/client';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

describe('getBranchChannels', () => {
  it('pins the branch independently of the selection and exposes only channel metadata', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [
        {
          id: 'c1',
          branch_id: 'b2',
          type: 'WHATSAPP',
          identifier: '+15556530731',
          is_active: false,
          is_configured: true,
          config: { access_token: 'must-not-reach-client' },
        },
      ],
    });
    await expect(getBranchChannels('b2')).resolves.toEqual([
      {
        id: 'c1',
        branchId: 'b2',
        type: 'WHATSAPP',
        identifier: '+15556530731',
        isActive: false,
        isConfigured: true,
      },
    ]);
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/channels',
      branchId: 'b2',
      query: { include_inactive: 'true' },
    });
  });
});
