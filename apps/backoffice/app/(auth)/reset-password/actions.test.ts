import { beforeEach, describe, expect, it, vi } from 'vitest';

import { resetPassword } from '@/app/(auth)/reset-password/actions';
import { type ResetPasswordValues } from '@/app/(auth)/reset-password/form-schema';

// Only the request: the error vocabulary is what turns a refusal into a code, and that is what
// this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

const VALUES: ResetPasswordValues = {
  token: 't0ken',
  newPassword: 'Coti-1234-larga',
  confirmPassword: 'Coti-1234-larga',
};

beforeEach(() => vi.clearAllMocks());

describe('resetPassword', () => {
  it('posts the token and the password to the public route with no bearer', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(resetPassword(VALUES)).resolves.toEqual({ done: true });
    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/public/auth/reset-password',
      method: 'POST',
      authenticated: false,
      body: { token: 't0ken', new_password: 'Coti-1234-larga' },
    });
  });

  // Only the policy belongs to the password field. Reading every 422 as one used to put "the
  // password is too short" under a field the caller had filled in correctly.
  it('lands a policy refusal on the password and nothing else there', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('PASSWORD_POLICY', 422));

    await expect(resetPassword(VALUES)).resolves.toEqual({
      error: 'PASSWORD_POLICY',
      field: 'newPassword',
    });
  });

  it('leaves any other 422 on the screen rather than on the password', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_INPUT', 422));

    await expect(resetPassword(VALUES)).resolves.toEqual({
      error: 'INVALID_INPUT',
      field: undefined,
    });
  });

  it('carries a rejected link through', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_LINK', 401));

    await expect(resetPassword(VALUES)).resolves.toEqual({
      error: 'INVALID_LINK',
      field: undefined,
    });
  });

  // The route carries its own allowance, and spending it used to read as "unexpected".
  it('reports the allowance running out as itself', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('RATE_LIMITED', 429));

    await expect(resetPassword(VALUES)).resolves.toEqual({
      error: 'RATE_LIMITED',
      field: undefined,
    });
  });

  it('never reaches the API with values its own schema refuses', async () => {
    await expect(resetPassword({ ...VALUES, confirmPassword: 'otra' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
