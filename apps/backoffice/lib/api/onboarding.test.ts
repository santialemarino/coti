import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getOnboarding } from '@/lib/api/onboarding';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

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
