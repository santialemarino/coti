import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getAccountBranches, getBranches } from '@/lib/api/branches';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

function rawBranch(overrides: Record<string, unknown> = {}) {
  return {
    id: 'b1',
    name: 'Villa Bosch',
    address: 'Av. Márquez 1234',
    default_expiry_days: 7,
    is_active: true,
    created_at: '2026-07-01T10:00:00Z',
    updated_at: '2026-07-02T10:00:00Z',
    ...overrides,
  };
}

beforeEach(() => vi.clearAllMocks());

describe('getBranches', () => {
  /*
   * The API speaks snake_case and the component tree speaks camelCase. This boundary is the
   * only place the two meet, so a field that is not mapped here surfaces as undefined deep in
   * a screen rather than as an error.
   */
  it('turns every snake_case field into its camelCase counterpart', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawBranch()] });

    await expect(getBranches()).resolves.toEqual([
      {
        id: 'b1',
        name: 'Villa Bosch',
        address: 'Av. Márquez 1234',
        defaultExpiryDays: 7,
        isActive: true,
      },
    ]);
  });

  // The timestamps are read but not surfaced; leaking them would let a screen render a raw
  // ISO string instead of going through the formatters.
  it('drops the fields no screen consumes', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawBranch()] });

    const [branch] = await getBranches();
    expect(branch).not.toHaveProperty('created_at');
    expect(branch).not.toHaveProperty('createdAt');
  });

  it('keeps a null address null rather than blanking it', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawBranch({ address: null })] });

    const branches = await getBranches();
    expect(branches[0]?.address).toBeNull();
  });

  it('preserves order and maps every row', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [rawBranch({ id: 'b1', name: 'Villa Bosch' }), rawBranch({ id: 'b2', name: 'Morón' })],
    });

    await expect(getBranches()).resolves.toMatchObject([
      { name: 'Villa Bosch' },
      { name: 'Morón' },
    ]);
  });

  /*
   * Collections come wrapped so a list can grow pagination without breaking its callers, which
   * is exactly the shape branches.ts once got wrong by typing the response as an array.
   *
   * And the list must never carry the active branch: a cookie the API would refuse turns this
   * into a 403, leaving the switcher that could fix it with nothing to show and no way back.
   */
  it('reads the items envelope, and asks without naming a branch', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [] });

    await expect(getBranches()).resolves.toEqual([]);
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/branches', branchScoped: false });
  });
});

describe('the two branch reads are deliberately separate', () => {
  /*
   * `getBranches` decides what the switcher offers and what `setActiveBranch` will accept. A closed
   * branch reaching either would pin the session to one the API refuses on every request, so this
   * read must never ask for the inactive ones — which is why the account-wide read is its own
   * function rather than a flag on this one.
   */
  it('never asks the API for closed branches on the read the switcher uses', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [] });

    await getBranches();

    const request = vi.mocked(apiRequest).mock.calls[0]?.[0] as { query?: Record<string, string> };
    expect(request.query).toBeUndefined();
  });

  it('asks for them on the read administration uses', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [] });

    await getAccountBranches();

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/branches',
      query: { include_inactive: 'true' },
      branchScoped: false,
    });
  });

  it('maps a closed branch as closed', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawBranch({ is_active: false })] });

    await expect(getAccountBranches()).resolves.toMatchObject([{ isActive: false }]);
  });
});
