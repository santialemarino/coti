import { beforeEach, describe, expect, it, vi } from 'vitest';

import ARCASettingsPage from '@/app/(protected)/settings/arca/page';
import { getARCASetup } from '@/lib/api/arca-setup';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('@/lib/api/arca-setup', () => ({ getARCASetup: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/utils/page', () => ({ generatePageMetadata: vi.fn() }));
beforeEach(() => vi.clearAllMocks());
describe('ARCA account settings', () => {
  it('loads the company connection independently of the active branch', async () => {
    vi.mocked(getARCASetup).mockResolvedValue({
      enabled: true,
      taxId: '20329642330',
      csr: 'request',
      hasCertificate: true,
      certificateExpiresAt: null,
      pointOfSale: 0,
      verifiedAt: null,
    });
    await ARCASettingsPage();
    expect(getARCASetup).toHaveBeenCalledWith();
  });
});
