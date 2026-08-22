import { beforeEach, describe, expect, it, vi } from 'vitest';

import { changeEmail } from '@/lib/auth/change-email';
import { type ChangeEmailValues } from '@/lib/auth/change-email-schema';

// Only the request and the session probe: what this file is about is which refusal lands on
// which field, and that no session is re-issued.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn(), startSession: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');
const { getSession, startSession } = await import('@/lib/auth/session');

const VALUES: ChangeEmailValues = {
  newEmail: 'ana.nueva@coti.test',
  currentPassword: 'coti1234',
};

const SESSION = {
  userId: 'u1',
  accountId: 'a1',
  name: 'Ana',
  email: 'ana@coti.test',
  emailVerified: false,
  role: 'ADMIN',
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getSession).mockResolvedValue(SESSION);
});

describe('changeEmail', () => {
  it('sends the address and the password under the API wire names', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(changeEmail(VALUES)).resolves.toEqual({ done: true });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/auth/change-email',
      method: 'POST',
      body: { new_email: VALUES.newEmail, current_password: VALUES.currentPassword },
    });
  });

  /*
   * Unlike a password change, this one is not a credential event: the sessions stand, so nothing
   * here writes cookies. Re-issuing a pair would be the tell that it revoked the old one.
   */
  it('opens no new session', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await changeEmail(VALUES);

    expect(startSession).not.toHaveBeenCalled();
  });

  it('lands a 401 on the current password while the session is still live', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNAUTHENTICATED', 401));

    await expect(changeEmail(VALUES)).resolves.toEqual({
      error: 'UNAUTHENTICATED',
      field: 'currentPassword',
    });
  });

  it('reports the same 401 as a lapsed session once the API no longer knows the caller', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNAUTHENTICATED', 401));
    vi.mocked(getSession).mockResolvedValue(null);

    await expect(changeEmail(VALUES)).resolves.toEqual({ error: 'SESSION_EXPIRED' });
  });

  /*
   * The API answers the same conflict whether a stranger holds the address or the caller already
   * does, so both land on the field they would edit either way.
   */
  it('lands a taken address on the address field', async () => {
    for (const code of ['EMAIL_TAKEN', 'CONFLICT'] as const) {
      vi.mocked(apiRequest).mockRejectedValue(new ApiError(code, 409));

      await expect(changeEmail(VALUES)).resolves.toEqual({ error: code, field: 'newEmail' });
    }
  });

  it('leaves a refusal that belongs to no field on the form', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INTERNAL', 500));

    await expect(changeEmail(VALUES)).resolves.toEqual({ error: 'INTERNAL', field: undefined });
  });

  it('never reaches the API with values its own schema refuses', async () => {
    await expect(changeEmail({ ...VALUES, newEmail: 'no-es-una-direccion' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
