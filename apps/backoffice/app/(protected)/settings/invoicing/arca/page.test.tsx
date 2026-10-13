import { beforeEach, describe, expect, it, vi } from 'vitest';

import ARCASettingsPage from '@/app/(protected)/settings/invoicing/arca/page';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('@/lib/api/arca-setup', () => ({ getARCASetup: vi.fn() }));
vi.mock('@/lib/api/invoicing-settings', () => ({ getInvoicingSettings: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/utils/page', () => ({ generatePageMetadata: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

describe('ARCA connection page', () => {
  it('hands the guide the account CUIT and branches alongside the connection', async () => {
    vi.mocked(getARCASetup).mockResolvedValue({
      enabled: true,
      taxId: '',
      csr: '',
      hasCertificate: false,
      certificate: '',
      certificateExpiresAt: null,
    });
    vi.mocked(getInvoicingSettings).mockResolvedValue({
      taxId: '30-71234567-9',
      branches: [{ branchId: 'b', name: 'Morón', isActive: true, pointOfSale: null }],
    } as never);

    const page = await ARCASettingsPage();
    const guide = (page.props.children as React.ReactElement[])[1] as React.ReactElement<{
      accountTaxId: string;
      branches: unknown[];
    }>;

    expect(requireAdmin).toHaveBeenCalled();
    expect(guide.props.accountTaxId).toBe('30-71234567-9');
    expect(guide.props.branches).toHaveLength(1);
  });
});
