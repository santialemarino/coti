import { beforeEach, describe, expect, it, vi } from 'vitest';

import { updateAccount } from '@/app/(protected)/settings/account/actions';
import { type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { ROUTES } from '@/config/routes';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
// Only the request: the error vocabulary is what maps a status onto a rejection, and that mapping
// is what this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { revalidatePath } = await import('next/cache');
const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

const VALUES: AccountValues = {
  name: 'Corralón San Martín',
  legalName: 'Corralón San Martín S.R.L.',
  taxId: '30-71234567-9',
  brandLogoUrl: 'https://tucorralon.com/logo.png',
  brandColor: '#C2410C',
};

function requestSent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0];
}

function bodySent() {
  return JSON.parse(JSON.stringify(requestSent()?.body));
}

beforeEach(() => vi.clearAllMocks());

describe('updateAccount', () => {
  it('puts the record and revalidates only this route', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(updateAccount(VALUES)).resolves.toEqual({ ok: true });
    expect(requestSent()).toMatchObject({ path: '/v1/account', method: 'PUT' });
    // No other screen renders the account, so revalidating the whole layout would refresh the
    // shell for nothing.
    expect(revalidatePath).toHaveBeenCalledWith(ROUTES.accountSettings);
  });

  it('sends the profile and the brand in snake_case', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await updateAccount(VALUES);

    expect(bodySent()).toEqual({
      name: 'Corralón San Martín',
      legal_name: 'Corralón San Martín S.R.L.',
      tax_id: '30-71234567-9',
      brand_logo_url: 'https://tucorralon.com/logo.png',
      brand_color: '#C2410C',
    });
  });

  /*
   * Omitting is how a value is cleared, and it has to be omission rather than an empty string: the
   * API's optional fields are pointers with `omitempty`, which only skips a nil one, so a pointer
   * to "" passes validation and lands in the column — "no logo" would become a logo of nothing.
   */
  it.each(['legalName', 'taxId', 'brandLogoUrl', 'brandColor'] as const)(
    'omits %s when it was emptied',
    async (field) => {
      vi.mocked(apiRequest).mockResolvedValue(undefined);

      await updateAccount({ ...VALUES, [field]: '' });

      const wireKey = {
        legalName: 'legal_name',
        taxId: 'tax_id',
        brandLogoUrl: 'brand_logo_url',
        brandColor: 'brand_color',
      }[field];
      expect(bodySent()).not.toHaveProperty(wireKey);
    },
  );

  it('never reaches the API with values its own schema refuses', async () => {
    await expect(updateAccount({ ...VALUES, name: '  ' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    await expect(updateAccount({ ...VALUES, brandColor: 'naranja' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    await expect(updateAccount({ ...VALUES, brandLogoUrl: 'tucorralon.com' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  // The code reaches the screen untouched, so the catalog is the only thing that decides what any
  // of them says.
  it.each([
    ['INVALID_BODY', 400],
    ['INVALID_INPUT', 422],
    ['FORBIDDEN', 403],
    ['NOT_FOUND', 404],
    ['INTERNAL', 500],
  ] as const)('hands %s back as it arrived', async (code, status) => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError(code, status));

    await expect(updateAccount(VALUES)).resolves.toEqual({ error: code });
  });

  it('leaves the tree alone when the write was refused', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INTERNAL', 500));

    await expect(updateAccount(VALUES)).resolves.toEqual({ error: 'INTERNAL' });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
