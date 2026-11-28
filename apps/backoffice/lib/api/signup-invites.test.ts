import { beforeEach, describe, expect, it, vi } from 'vitest';

import { apiRequest } from '@/lib/api/client';
import { ApiError } from '@/lib/api/errors';
import { signupInviteRefusal } from '@/lib/api/signup-invites';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

beforeEach(() => vi.resetAllMocks());

describe('signupInviteRefusal', () => {
  it('allows signup with a valid invitation', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);
    await expect(signupInviteRefusal('invite')).resolves.toBeNull();
  });

  it.each(['SIGNUP_INVITE_INVALID', 'SIGNUP_INVITE_USED', 'SIGNUP_INVITE_EXPIRED'] as const)(
    'returns the invitation refusal %s',
    async (code) => {
      vi.mocked(apiRequest).mockRejectedValue(new ApiError(code, 403));
      await expect(signupInviteRefusal('invite')).resolves.toBe(code);
    },
  );

  it.each([
    new ApiError('RATE_LIMITED', 429),
    new ApiError('INTERNAL', 500),
    new ApiError('UNREACHABLE', 0),
  ])('propagates $code to the auth error boundary', async (error) => {
    vi.mocked(apiRequest).mockRejectedValue(error);
    await expect(signupInviteRefusal('invite')).rejects.toBe(error);
  });
});
