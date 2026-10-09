import { beforeEach, describe, expect, it, vi } from 'vitest';

import { checkARCAConnection, getARCASetup } from '@/lib/api/arca-setup';
import { apiRequest } from '@/lib/api/client';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
beforeEach(() => vi.resetAllMocks());

describe('ARCA setup boundary', () => {
  it('maps public metadata without carrying private credentials', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      enabled: true,
      tax_id: '20329642330',
      csr: 'public request',
      has_certificate: true,
      certificate_expires_at: '2027-01-01',
      point_of_sale: 7,
      verified_at: '2026-10-09',
    });
    await expect(getARCASetup('selected-branch')).resolves.toEqual({
      enabled: true,
      taxId: '20329642330',
      csr: 'public request',
      hasCertificate: true,
      certificateExpiresAt: '2027-01-01',
      pointOfSale: 7,
      verifiedAt: '2026-10-09',
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/arca/setup',
      branchId: 'selected-branch',
    });
  });
  it('pins verification to the branch where it was prepared', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      verified: false,
      failure: 'POINT_OF_SALE_REJECTED',
      last_number: 0,
    });
    await expect(checkARCAConnection(7, 'selected-branch')).resolves.toEqual({
      verified: false,
      failure: 'POINT_OF_SALE_REJECTED',
      lastNumber: 0,
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/arca/setup/verify',
      method: 'POST',
      body: { point_of_sale: 7 },
      branchId: 'selected-branch',
    });
  });
});
