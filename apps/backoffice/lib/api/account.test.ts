import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getAccount } from '@/lib/api/account';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

function rawAccount(overrides: Record<string, unknown> = {}) {
  return {
    id: 'a1',
    name: 'Corralón San Martín',
    legal_name: 'Corralón San Martín S.R.L.',
    tax_id: '30-71234567-9',
    brand_logo_url: 'https://tucorralon.com/logo.png',
    brand_color: '#C2410C',
    is_active: true,
    created_at: '2026-07-01T10:00:00Z',
    updated_at: '2026-07-02T10:00:00Z',
    ...overrides,
  };
}

beforeEach(() => vi.clearAllMocks());

describe('getAccount', () => {
  /*
   * The API speaks snake_case and the component tree speaks camelCase. This boundary is the only
   * place the two meet, so a field that is not mapped here surfaces as undefined deep in a screen
   * rather than as an error.
   */
  it('turns every snake_case field into its camelCase counterpart', async () => {
    vi.mocked(apiRequest).mockResolvedValue(rawAccount());

    await expect(getAccount()).resolves.toEqual({
      id: 'a1',
      name: 'Corralón San Martín',
      legalName: 'Corralón San Martín S.R.L.',
      taxId: '30-71234567-9',
      brandLogoUrl: 'https://tucorralon.com/logo.png',
      brandColor: '#C2410C',
    });
  });

  /*
   * The timestamps would let a screen render a raw ISO string instead of going through the
   * formatters, and `is_active` is a flag no caller can ever see as false: an inactive account
   * cannot hold a session, so a form offering to toggle it would offer a lockout.
   */
  it('drops the fields no screen consumes', async () => {
    vi.mocked(apiRequest).mockResolvedValue(rawAccount());

    const account = await getAccount();
    expect(account).not.toHaveProperty('createdAt');
    expect(account).not.toHaveProperty('created_at');
    expect(account).not.toHaveProperty('isActive');
    expect(account).not.toHaveProperty('is_active');
  });

  // Null is what "never set" looks like, and the form turns it into the empty string a text input
  // holds. Blanking it here would lose the difference for anything else that reads this.
  it('keeps an unset optional field null', async () => {
    vi.mocked(apiRequest).mockResolvedValue(
      rawAccount({ legal_name: null, tax_id: null, brand_logo_url: null, brand_color: null }),
    );

    await expect(getAccount()).resolves.toMatchObject({
      legalName: null,
      taxId: null,
      brandLogoUrl: null,
      brandColor: null,
    });
  });

  // A single record, not an items envelope — the account is the tenant, so there is never a list.
  it('reads the record straight off the route', async () => {
    vi.mocked(apiRequest).mockResolvedValue(rawAccount());

    await getAccount();

    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/account' });
  });
});
