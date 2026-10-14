import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getARCASetup } from '@/lib/api/arca-setup';
import { apiRequest } from '@/lib/api/client';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
beforeEach(() => vi.resetAllMocks());

describe('ARCA setup boundary', () => {
  it('maps the public material, certificate included, for the whole account', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      enabled: true,
      tax_id: '20329642330',
      csr: 'public request',
      has_certificate: true,
      certificate: 'public certificate',
      certificate_expires_at: '2027-01-01',
    });

    await expect(getARCASetup()).resolves.toEqual({
      enabled: true,
      taxId: '20329642330',
      csr: 'public request',
      hasCertificate: true,
      certificate: 'public certificate',
      certificateExpiresAt: '2027-01-01',
    });
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/arca/setup', branchScoped: false });
  });
});
