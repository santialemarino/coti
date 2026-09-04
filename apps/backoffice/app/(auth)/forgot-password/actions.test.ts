import { beforeEach, describe, expect, it, vi } from 'vitest';

import { requestPasswordRecovery } from '@/app/(auth)/forgot-password/actions';

// Only the request: the error vocabulary is what turns a refusal into a code, and that is what
// this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

const VALUES = { email: 'ana@corralonsanmartin.test' };

beforeEach(() => vi.clearAllMocks());

describe('requestPasswordRecovery', () => {
  it('posts to the public route with no bearer', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(requestPasswordRecovery(VALUES)).resolves.toEqual({ sent: true });
    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/public/auth/forgot-password',
      method: 'POST',
      authenticated: false,
    });
  });

  /*
   * The screen answers the same whether or not the address is registered, and the per-address cap
   * answers 202 for the same reason. The 429 that does arrive is the caller's own allowance, and
   * it used to fall through to "Ocurrió un error inesperado" with nothing to act on.
   */
  it('reports the caller allowance running out as itself', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('RATE_LIMITED', 429));

    await expect(requestPasswordRecovery(VALUES)).resolves.toEqual({
      error: 'RATE_LIMITED',
      field: undefined,
    });
  });

  it('lands a rejected body on the address, which is the only field there is', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_BODY', 400));

    await expect(requestPasswordRecovery(VALUES)).resolves.toEqual({
      error: 'INVALID_BODY',
      field: 'email',
    });
  });

  it('never reaches the API with an address its own schema refuses', async () => {
    await expect(requestPasswordRecovery({ email: 'nope' })).resolves.toEqual({
      error: 'INVALID_BODY',
      field: 'email',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
