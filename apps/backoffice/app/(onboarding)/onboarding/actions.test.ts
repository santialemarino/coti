import { beforeEach, describe, expect, it, vi } from 'vitest';

import { updateOnboardingBrand } from '@/app/(onboarding)/onboarding/actions';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/account', () => ({
  getAccount: vi.fn(() =>
    Promise.resolve({
      name: 'Corralón Norte',
      legalName: null,
      taxId: null,
      brandLogoUrl: null,
    }),
  ),
}));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('updateOnboardingBrand', () => {
  it('adds the fixed hash prefix at the API boundary', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(updateOnboardingBrand({ brandColor: 'C2410C' })).resolves.toEqual({ ok: true });
    expect(apiRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        body: expect.objectContaining({ brand_color: '#C2410C' }),
      }),
    );
  });

  it('does not send an empty optional colour', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await updateOnboardingBrand({ brandColor: '' });

    expect(apiRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        body: expect.not.objectContaining({ brand_color: expect.anything() }),
      }),
    );
  });
});
