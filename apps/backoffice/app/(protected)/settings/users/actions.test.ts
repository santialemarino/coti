import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createUser,
  deactivateUser,
  reactivateUser,
  sendPasswordReset,
  updateUser,
} from '@/app/(protected)/settings/users/actions';
import { type UserValues } from '@/app/(protected)/settings/users/form-schema';
import { ADMIN_ROLE, SELLER_ROLE } from '@/lib/constants/auth';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/password';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
// Only the request: the error vocabulary is what maps a status onto a rejection, and that mapping
// is what this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { revalidatePath } = await import('next/cache');
const { ApiError, apiRequest } = await import('@/lib/api/client');

const USER_ID = 'u0000000-0000-4000-8000-000000000001';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const VALUES: UserValues = {
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: SELLER_ROLE,
  branchIds: [BRANCH_ID],
  password: 'Coti-1234-larga',
};

function requestSent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0];
}

beforeEach(() => vi.clearAllMocks());

describe('createUser', () => {
  it('posts the user and revalidates the layout', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(createUser(VALUES)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: '/v1/users', method: 'POST' });
    // An admin editing their own name changes the shell's account menu, and the shell is not on
    // this route's tree.
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });

  it('sends the profile in snake_case, with the initial password', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await createUser(VALUES);

    expect(requestSent()?.body).toEqual({
      name: 'Ana Gómez',
      email: 'ana@corralon.test',
      role: SELLER_ROLE,
      branch_ids: [BRANCH_ID],
      password: 'Coti-1234-larga',
    });
  });

  it('never reaches the API with values its own schema refuses', async () => {
    await expect(
      createUser({ ...VALUES, password: 'a'.repeat(PASSWORD_MIN_LENGTH - 1) }),
    ).resolves.toEqual({ error: 'invalid' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  // The address is unique across every account since the global constraint, so a 409 is the one
  // rejection that belongs to a field rather than to the list.
  it('reads a 409 as an address already in use', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('conflict', 409));

    await expect(createUser(VALUES)).resolves.toEqual({ error: 'emailTaken' });
  });

  /*
   * Creating cannot hit a self-edit guard, so a 422 here means this form and the API's own
   * validation have drifted apart — or a branch closed between the page load and the submit.
   */
  it('reads a 422 as a validation problem', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(createUser(VALUES)).resolves.toEqual({ error: 'invalid' });
  });

  it('leaves the tree alone when the write was refused', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('conflict', 409));

    await createUser(VALUES);

    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe('updateUser', () => {
  it('puts to the user it was given', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(updateUser(USER_ID, VALUES)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: `/v1/users/${USER_ID}`, method: 'PUT' });
  });

  /*
   * The update body carries no password at all: an admin who has to change one mails a recovery
   * link, so nobody but the user ever sets it. A password reaching this route would be silently
   * ignored, which is worse than being refused.
   */
  it('never sends a password, even when the form holds one', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await updateUser(USER_ID, VALUES);

    expect(requestSent()?.body).not.toHaveProperty('password');
  });

  // The API replaces the whole assignment set, so an omitted list would read as "leave them alone"
  // and a seller could never be stripped of their last branch.
  it('sends an empty assignment list rather than omitting it', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await updateUser(USER_ID, { ...VALUES, branchIds: [] });

    expect(requestSent()?.body).toMatchObject({ branch_ids: [] });
  });

  it('leaves the active flag alone', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await updateUser(USER_ID, VALUES);

    expect(requestSent()?.body).not.toHaveProperty('is_active');
  });

  it('reports a user that is gone', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('notFound', 404));

    await expect(updateUser(USER_ID, VALUES)).resolves.toEqual({ error: 'notFound' });
  });

  it('reads a 409 as an address already in use', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('conflict', 409));

    await expect(updateUser(USER_ID, VALUES)).resolves.toEqual({ error: 'emailTaken' });
  });
});

describe('deactivateUser', () => {
  it('deletes the user and revalidates the layout', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(deactivateUser(USER_ID)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: `/v1/users/${USER_ID}`, method: 'DELETE' });
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });

  /*
   * The one 422 this route answers, and the reason it gets its own message: an admin who
   * deactivates themselves has no way back into the account. The API answers 422 and not 403,
   * which is the mistake to avoid — the list hides the action, so seeing this means the two
   * disagree.
   */
  it('names the self-deactivation guard when the API refuses', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(deactivateUser(USER_ID)).resolves.toEqual({ error: 'self' });
  });

  it('reads a 403 as a session without the role, not as the guard', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('forbidden', 403));

    await expect(deactivateUser(USER_ID)).resolves.toEqual({ error: 'unauthorized' });
  });
});

describe('reactivateUser', () => {
  const DEACTIVATED = {
    id: USER_ID,
    name: 'Ana Gómez',
    email: 'ana@corralon.test',
    role: ADMIN_ROLE,
    branchIds: [BRANCH_ID],
  };

  /*
   * `PUT` replaces the record, so reactivating has to carry the profile it already has alongside
   * the flag — sending the flag alone would fail the API's own validation.
   */
  it('replaces the record with the flag turned back on', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(reactivateUser(DEACTIVATED)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: `/v1/users/${USER_ID}`, method: 'PUT' });
    expect(requestSent()?.body).toEqual({
      name: 'Ana Gómez',
      email: 'ana@corralon.test',
      role: ADMIN_ROLE,
      branch_ids: [BRANCH_ID],
      is_active: true,
    });
  });
});

describe('sendPasswordReset', () => {
  it('asks the API to mail the link', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(sendPasswordReset(USER_ID)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({
      path: `/v1/users/${USER_ID}/password-reset`,
      method: 'POST',
    });
  });

  // The only 422 this route answers: a deactivated user gets no recovery link, because the link
  // would let them back in without reactivating them.
  it('names the deactivated user when the API refuses to mail one', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(sendPasswordReset(USER_ID)).resolves.toEqual({ error: 'deactivated' });
  });

  // The route shares the mail allowance, so an admin resetting several users in a row is the one
  // place in this screen a 429 is expected rather than abusive.
  it('reports the mail allowance running out', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('rateLimited', 429));

    await expect(sendPasswordReset(USER_ID)).resolves.toEqual({ error: 'rateLimited' });
  });
});
