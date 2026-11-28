import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getOnboarding, getOperationalReadiness } from '@/lib/api/onboarding';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

it('maps current readiness without exposing raw field names', async () => {
  vi.mocked(apiRequest).mockResolvedValue({
    account_ready: true,
    branches: [
      {
        id: 'branch-1',
        name: 'Centro',
        products: 3,
        missing_prices: 2,
        has_staff: true,
        has_channel: false,
      },
    ],
  });
  await expect(getOperationalReadiness()).resolves.toEqual({
    accountReady: true,
    branches: [
      {
        id: 'branch-1',
        name: 'Centro',
        products: 3,
        missingPrices: 2,
        hasStaff: true,
        hasChannel: false,
      },
    ],
  });
  expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/onboarding/readiness' });
});

it('propagates diagnostic failures instead of reporting completion', async () => {
  vi.mocked(apiRequest).mockRejectedValue(new Error('unavailable'));
  await expect(getOperationalReadiness()).rejects.toThrow('unavailable');
});

describe('getOnboarding', () => {
  it('maps the versioned API state without translating stable step keys', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      flow_version: 1,
      status: 'IN_PROGRESS',
      current_step: 'FIRST_BRANCH',
      steps: { WELCOME: 'COMPLETED', BRAND: 'SKIPPED' },
      completed_at: null,
    });

    await expect(getOnboarding()).resolves.toEqual({
      flowVersion: 1,
      status: 'IN_PROGRESS',
      currentStep: 'FIRST_BRANCH',
      steps: { WELCOME: 'COMPLETED', BRAND: 'SKIPPED' },
      completedAt: null,
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/onboarding',
      branchScoped: false,
    });
  });
});
