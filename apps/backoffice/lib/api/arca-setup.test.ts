import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getARCASetup } from '@/lib/api/arca-setup';
import { apiRequest } from '@/lib/api/client';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
beforeEach(() => vi.resetAllMocks());

describe('ARCA setup boundary', () => {
  it('maps the public material, certificate included, for the whole account', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      enabled: true,
      environment: 'PRODUCTION',
      setup_environment: 'HOMOLOGATION',
      reconnect_required: true,
      tax_id: '20329642330',
      csr: 'public request',
      has_certificate: true,
      certificate: 'public certificate',
      certificate_expires_at: '2027-01-01',
    });

    await expect(getARCASetup()).resolves.toEqual({
      enabled: true,
      environment: 'PRODUCTION',
      setupEnvironment: 'HOMOLOGATION',
      reconnectRequired: true,
      taxId: '20329642330',
      csr: 'public request',
      hasCertificate: true,
      certificate: 'public certificate',
      certificateExpiresAt: '2027-01-01',
    });
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/arca/setup', branchScoped: false });
  });

  it('reads no connection as no environment of its own', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      enabled: true,
      environment: 'HOMOLOGATION',
      setup_environment: null,
      reconnect_required: false,
      tax_id: '',
      csr: '',
      has_certificate: false,
      certificate: '',
      certificate_expires_at: null,
    });

    const setup = await getARCASetup();
    expect(setup.environment).toBe('HOMOLOGATION');
    expect(setup.setupEnvironment).toBeNull();
    expect(setup.reconnectRequired).toBe(false);
  });
});
