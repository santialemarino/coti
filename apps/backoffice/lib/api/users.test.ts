import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getUsers } from '@/lib/api/users';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

function rawUser(overrides: Record<string, unknown> = {}) {
  return {
    id: 'u1',
    name: 'Ana Gómez',
    email: 'ana@corralon.test',
    role: 'SELLER',
    is_active: true,
    branch_ids: ['b1', 'b2'],
    last_login_at: '2026-08-01T13:05:00Z',
    created_at: '2026-07-01T10:00:00Z',
    updated_at: '2026-07-02T10:00:00Z',
    ...overrides,
  };
}

beforeEach(() => vi.clearAllMocks());

describe('getUsers', () => {
  /*
   * The API speaks snake_case and the component tree speaks camelCase. This boundary is the only
   * place the two meet, so a field that is not mapped here surfaces as undefined deep in a screen
   * rather than as an error.
   */
  it('turns every snake_case field into its camelCase counterpart', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawUser()] });

    await expect(getUsers()).resolves.toEqual([
      {
        id: 'u1',
        name: 'Ana Gómez',
        email: 'ana@corralon.test',
        role: 'SELLER',
        isActive: true,
        branchIds: ['b1', 'b2'],
        lastLoginAt: '2026-08-01T13:05:00Z',
      },
    ]);
  });

  // The timestamps are read but not surfaced; leaking them would let a screen render a raw ISO
  // string instead of going through the formatters.
  it('drops the fields no screen consumes', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawUser()] });

    const [user] = await getUsers();
    expect(user).not.toHaveProperty('created_at');
    expect(user).not.toHaveProperty('createdAt');
    expect(user).not.toHaveProperty('password_hash');
  });

  // Null is what a user who has never logged in looks like, and the screen says so. Blanking it
  // would read as a login at the epoch.
  it('keeps an absent last login null', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawUser({ last_login_at: null })] });

    const users = await getUsers();
    expect(users[0]?.lastLoginAt).toBeNull();
  });

  it('maps a deactivated user as deactivated', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawUser({ is_active: false })] });

    await expect(getUsers()).resolves.toMatchObject([{ isActive: false }]);
  });

  // Collections come wrapped so a list can grow pagination without breaking its callers.
  it('reads the items envelope and preserves the order the API sent', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [rawUser({ id: 'u1', name: 'Ana Gómez' }), rawUser({ id: 'u2', name: 'Bruno Díaz' })],
    });

    await expect(getUsers()).resolves.toMatchObject([
      { name: 'Ana Gómez' },
      { name: 'Bruno Díaz' },
    ]);
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/users' });
  });

  it('answers an empty account with an empty list', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [] });

    await expect(getUsers()).resolves.toEqual([]);
  });
});
