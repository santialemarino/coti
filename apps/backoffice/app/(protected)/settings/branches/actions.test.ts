import { beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import {
  closeBranch,
  createBranch,
  reopenBranch,
  updateBranch,
} from '@/app/(protected)/settings/branches/actions';
import { type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
// Only the request: the error vocabulary is what maps a status onto a rejection, and that mapping
// is what this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { cookies } = await import('next/headers');
const { revalidatePath } = await import('next/cache');
const { ApiError, apiRequest } = await import('@/lib/api/client');

const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const VALUES: BranchValues = {
  name: 'Casa Central',
  address: 'Av. Siempre Viva 742',
  defaultExpiryDays: '14',
};

function jar(initial: Record<string, string> = {}) {
  const fake = cookieJar(initial);
  vi.mocked(cookies).mockResolvedValue(fake as unknown as Awaited<ReturnType<typeof cookies>>);
  return fake;
}

function requestSent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0];
}

beforeEach(() => vi.clearAllMocks());

describe('createBranch', () => {
  it('posts the branch and revalidates the layout', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(createBranch(VALUES)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: '/v1/branches', method: 'POST' });
    // The shell's switcher lists branches too, and it is not on this route's tree.
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });

  it('sends the expiry as a number, not the string the form holds', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await createBranch(VALUES);

    expect(requestSent()?.body).toEqual({
      name: 'Casa Central',
      address: 'Av. Siempre Viva 742',
      default_expiry_days: 14,
    });
  });

  /*
   * The API's optional fields are pointers with `omitempty`, which only skips a nil one — a pointer
   * to "" passes validation and lands in the column, so "no address" would become an empty one.
   */
  it('omits a blank address rather than sending an empty string', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await createBranch({ ...VALUES, address: '' });

    expect(JSON.parse(JSON.stringify(requestSent()?.body))).not.toHaveProperty('address');
  });

  it('never reaches the API with values its own schema refuses', async () => {
    jar();

    await expect(createBranch({ ...VALUES, name: '  ' })).resolves.toEqual({ error: 'invalid' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  // Creating cannot hit the last-active refusal, so a 422 here means this form and the API's own
  // validation have drifted apart — not something to explain with a branch-count message.
  it('reads a 422 as a validation problem, not as the last active branch', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(createBranch(VALUES)).resolves.toEqual({ error: 'invalid' });
  });
});

describe('updateBranch', () => {
  it('puts to the branch it was given', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(updateBranch(BRANCH_ID, VALUES)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({
      path: `/v1/branches/${BRANCH_ID}`,
      method: 'PUT',
    });
  });

  it('reports a branch that is gone', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('notFound', 404));

    await expect(updateBranch(BRANCH_ID, VALUES)).resolves.toEqual({ error: 'notFound' });
  });
});

describe('closeBranch', () => {
  it('deletes the branch and revalidates the layout', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(closeBranch(BRANCH_ID)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({
      path: `/v1/branches/${BRANCH_ID}`,
      method: 'DELETE',
    });
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });

  // The one 422 this route answers, and the whole reason it gets its own message: an account has to
  // keep somewhere to operate.
  it('names the last active branch when the API refuses to close it', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(closeBranch(BRANCH_ID)).resolves.toEqual({ error: 'lastActive' });
  });

  /*
   * The API refuses a branch that is not active, so a selection pointing at the branch just closed
   * would answer 403 on every branch-scoped read and lock the caller out of the app until they
   * happened to notice the switcher.
   */
  it('drops the selection when the branch it closed was the active one', async () => {
    const store = jar({ [BRANCH_COOKIE]: BRANCH_ID });
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await closeBranch(BRANCH_ID);

    expect(store.delete).toHaveBeenCalledWith(BRANCH_COOKIE);
  });

  it('leaves a selection naming a different branch alone', async () => {
    const store = jar({ [BRANCH_COOKIE]: 'b0000000-0000-4000-8000-000000000002' });
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await closeBranch(BRANCH_ID);

    expect(store.delete).not.toHaveBeenCalled();
  });

  it('keeps the selection when the close was refused', async () => {
    const store = jar({ [BRANCH_COOKIE]: BRANCH_ID });
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await closeBranch(BRANCH_ID);

    expect(store.delete).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe('reopenBranch', () => {
  const CLOSED = {
    id: BRANCH_ID,
    name: 'Morón',
    address: 'Rivadavia 18400',
    defaultExpiryDays: 5,
  };

  /*
   * `PUT` replaces the record, so reopening has to carry the branch's current name and expiry
   * alongside the flag — sending the flag alone would fail the API's own validation.
   */
  it('replaces the record with the flag turned back on', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(reopenBranch(CLOSED)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: `/v1/branches/${BRANCH_ID}`, method: 'PUT' });
    expect(requestSent()?.body).toEqual({
      name: 'Morón',
      address: 'Rivadavia 18400',
      default_expiry_days: 5,
      is_active: true,
    });
  });

  it('omits an absent address rather than sending null', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await reopenBranch({ ...CLOSED, address: null });

    expect(JSON.parse(JSON.stringify(requestSent()?.body))).not.toHaveProperty('address');
  });

  // Reopening never touches the selection: the branch was not the active one, because a closed
  // branch cannot be selected in the first place.
  it('leaves the selection alone', async () => {
    const store = jar({ [BRANCH_COOKIE]: BRANCH_ID });
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await reopenBranch(CLOSED);

    expect(store.delete).not.toHaveBeenCalled();
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });
});
