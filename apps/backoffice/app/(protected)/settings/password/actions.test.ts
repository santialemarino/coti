import { beforeEach, describe, expect, it, vi } from 'vitest';

import { changePassword } from '@/app/(protected)/settings/password/actions';
import { type ChangePasswordValues } from '@/app/(protected)/settings/password/form-schema';

// Only the request and the session probe: the error vocabulary is what turns a refusal into a
// code, and that is what this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));
vi.mock('@/lib/auth/session', () => ({
  getSession: vi.fn(),
  isRemembered: vi.fn(),
  startSession: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');
const { getSession, isRemembered, startSession } = await import('@/lib/auth/session');

const VALUES: ChangePasswordValues = {
  currentPassword: 'coti1234',
  newPassword: 'Coti-1234-larga',
  confirmPassword: 'Coti-1234-larga',
};

const SESSION = {
  userId: 'u1',
  accountId: 'a1',
  name: 'Ana',
  email: 'ana@coti.test',
  role: 'ADMIN',
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(isRemembered).mockResolvedValue(false);
  vi.mocked(getSession).mockResolvedValue(SESSION);
});

describe('changePassword', () => {
  /*
   * The change ends every session the user holds, this one included, so the pair the API hands
   * back has to be persisted here — otherwise the caller is logged out by their own change.
   */
  it('persists the fresh pair the API answers with', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ access_token: 'a', refresh_token: 'r' });

    await expect(changePassword(VALUES)).resolves.toEqual({ done: true });
    expect(startSession).toHaveBeenCalledWith({ accessToken: 'a', refreshToken: 'r' }, false);
  });

  /*
   * The route answers 401 for a wrong current password and for a bearer the API no longer honours,
   * and only asking again tells them apart: telling a user their password is wrong when their
   * session simply lapsed sends them chasing the wrong problem.
   */
  it('lands a 401 on the current password while the session is still live', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNAUTHENTICATED', 401));

    await expect(changePassword(VALUES)).resolves.toEqual({
      error: 'UNAUTHENTICATED',
      field: 'currentPassword',
    });
  });

  it('reports the same 401 as a lapsed session once the API no longer knows the caller', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNAUTHENTICATED', 401));
    vi.mocked(getSession).mockResolvedValue(null);

    await expect(changePassword(VALUES)).resolves.toEqual({ error: 'SESSION_EXPIRED' });
  });

  it('lands a policy refusal on the new password', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('PASSWORD_POLICY', 422));

    await expect(changePassword(VALUES)).resolves.toEqual({
      error: 'PASSWORD_POLICY',
      field: 'newPassword',
    });
  });

  it('leaves a refusal that belongs to no field on the form', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INTERNAL', 500));

    await expect(changePassword(VALUES)).resolves.toEqual({
      error: 'INTERNAL',
      field: undefined,
    });
    expect(startSession).not.toHaveBeenCalled();
  });

  it('never reaches the API with values its own schema refuses', async () => {
    await expect(changePassword({ ...VALUES, confirmPassword: 'otra' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
