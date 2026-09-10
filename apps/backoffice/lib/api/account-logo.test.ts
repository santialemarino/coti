import { beforeEach, describe, expect, it, vi } from 'vitest';

import { uploadAccountLogo } from '@/lib/api/account-logo';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
vi.mock('@/lib/config', () => ({ API_URL: 'https://api.coti.test' }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('uploadAccountLogo', () => {
  it('posts the file and resolves the returned public path against the API', async () => {
    const logo = new File(['logo'], 'logo.png', { type: 'image/png' });
    vi.mocked(apiRequest).mockResolvedValue({
      path: '/v1/public/account-logos/a1/l1',
    });

    await expect(uploadAccountLogo(logo)).resolves.toBe(
      'https://api.coti.test/v1/public/account-logos/a1/l1',
    );
    const request = vi.mocked(apiRequest).mock.calls[0]?.[0];
    expect(request).toMatchObject({
      path: '/v1/account/logo',
      method: 'POST',
      branchScoped: false,
    });
    expect(request?.formData?.get('file')).toBe(logo);
  });
});
