import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  setInvoicingEnabled,
  updateInvoicingSettings,
  updatePointsOfSale,
} from '@/app/(protected)/settings/invoicing/actions';
import type { InvoicingSettingsValues } from '@/app/(protected)/settings/invoicing/form-schema';
import { ApiError } from '@/lib/api/errors';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

const SETTINGS_RAW = {
  legal_name: 'Corralón del Sur SRL',
  tax_id: '30711111112',
  iva_condition: 'REGISTERED',
  prices_include_vat: true,
  branches: [],
};

const VALUES: InvoicingSettingsValues = {
  ivaCondition: 'REGISTERED',
  pricesIncludeVat: true,
  address: 'Av.  Rivadavia 1234',
  grossIncomeKind: 'number',
  grossIncomeNumber: '901.123456-7',
  activityStartedOn: '2020-01-02',
  branches: [
    { branchId: 'b1', pointOfSale: '3' },
    { branchId: 'b2', pointOfSale: '' },
  ],
};

beforeEach(() => vi.clearAllMocks());

describe('updateInvoicingSettings', () => {
  // Every branch is sent, so a cleared number reaches the API as null and is saved with the rest.
  it('maps the form to the API contract, points of sale included', async () => {
    vi.mocked(apiRequest).mockResolvedValue(SETTINGS_RAW);

    const result = await updateInvoicingSettings(VALUES);

    expect(result.settings?.legalName).toBe('Corralón del Sur SRL');
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/invoicing/settings',
      method: 'PUT',
      branchScoped: false,
      body: {
        iva_condition: 'REGISTERED',
        prices_include_vat: true,
        branches: [
          { branch_id: 'b1', point_of_sale: 3 },
          { branch_id: 'b2', point_of_sale: null },
        ],
        profile: {
          address: 'Av.  Rivadavia 1234',
          gross_income_registration: '901123456-7',
          activity_started_on: '2020-01-02',
        },
      },
    });
  });

  it('sends an exempt issuer as the canonical value', async () => {
    vi.mocked(apiRequest).mockResolvedValue(SETTINGS_RAW);

    await updateInvoicingSettings({ ...VALUES, grossIncomeKind: 'exempt' });

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      body: { profile: { gross_income_registration: 'EXENTO' } },
    });
  });

  it('refuses an unset IVA condition before calling the API', async () => {
    await expect(updateInvoicingSettings({ ...VALUES, ivaCondition: '' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('refuses two branches on one point of sale before calling the API', async () => {
    const shared = {
      ...VALUES,
      branches: [
        { branchId: 'b1', pointOfSale: '3' },
        { branchId: 'b2', pointOfSale: '03' },
      ],
    };

    await expect(updateInvoicingSettings(shared)).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it.each(['0', '99999'])('refuses a point of sale outside ARCA’s range (%s)', async (value) => {
    const outside = { ...VALUES, branches: [{ branchId: 'b1', pointOfSale: value }] };

    await expect(updateInvoicingSettings(outside)).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('returns the API’s code', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_INPUT', 422));

    await expect(updateInvoicingSettings(VALUES)).resolves.toEqual({ error: 'INVALID_INPUT' });
  });
});

describe('updatePointsOfSale', () => {
  it('sends only the points of sale, to their own route', async () => {
    vi.mocked(apiRequest).mockResolvedValue(SETTINGS_RAW);

    await updatePointsOfSale([
      { branchId: 'b1', pointOfSale: '2' },
      { branchId: 'b2', pointOfSale: '' },
    ]);

    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/invoicing/points-of-sale',
      method: 'PUT',
      branchScoped: false,
      body: {
        branches: [
          { branch_id: 'b1', point_of_sale: 2 },
          { branch_id: 'b2', point_of_sale: null },
        ],
      },
    });
  });

  it('refuses a duplicate before calling the API', async () => {
    await expect(
      updatePointsOfSale([
        { branchId: 'b1', pointOfSale: '2' },
        { branchId: 'b2', pointOfSale: '2' },
      ]),
    ).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});

describe('setInvoicingEnabled', () => {
  // The switch changes what the whole shell shows, so every protected layout is re-read.
  it('sends the switch and revalidates every screen', async () => {
    vi.mocked(apiRequest).mockResolvedValue({});
    const { revalidatePath } = await import('next/cache');

    await expect(setInvoicingEnabled(false)).resolves.toEqual({});

    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/invoicing/enabled',
      method: 'PUT',
      branchScoped: false,
      body: { enabled: false },
    });
    expect(revalidatePath).toHaveBeenCalledWith('/', 'layout');
  });

  it('returns the code a refusal carries', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVOICE_PENDING', 409));

    await expect(setInvoicingEnabled(false)).resolves.toEqual({ error: 'INVOICE_PENDING' });
  });
});
