import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  deleteArcaCredentials,
  updateInvoicingSettings,
  uploadArcaCredentials,
} from '@/app/(protected)/settings/invoicing/actions';
import type { InvoicingSettingsValues } from '@/app/(protected)/settings/invoicing/form-schema';
import { ApiError } from '@/lib/api/errors';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

const SETTINGS_RAW = {
  enabled: true,
  environment: 'homologation',
  legal_name: 'Corralón del Sur SRL',
  tax_id: '30711111112',
  iva_condition: 'REGISTERED',
  prices_include_vat: true,
  branches: [],
  credential: null,
};

const VALUES: InvoicingSettingsValues = {
  ivaCondition: 'REGISTERED',
  pricesIncludeVat: true,
  branches: [
    { branchId: 'b1', pointOfSale: '3' },
    { branchId: 'b2', pointOfSale: '' },
  ],
};

beforeEach(() => vi.clearAllMocks());

describe('updateInvoicingSettings', () => {
  it('maps the form to the API contract', async () => {
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
      },
    });
  });

  // An unset condition is a null on the wire, never the empty string the Combobox holds.
  it('sends an unset condition as null', async () => {
    vi.mocked(apiRequest).mockResolvedValue(SETTINGS_RAW);

    await updateInvoicingSettings({ ...VALUES, ivaCondition: '', pricesIncludeVat: false });

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      body: { iva_condition: null, prices_include_vat: false },
    });
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

describe('ARCA credentials', () => {
  it('uploads the certificate and the key as the two multipart fields', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      cuit: '30711111112',
      subject: 'CN=coti',
      expires_at: '2028-01-01T00:00:00Z',
      updated_at: '2026-10-05T12:00:00Z',
    });
    const form = new FormData();
    form.set('certificate', new File(['cert'], 'coti.crt'));
    form.set('private_key', new File(['key'], 'coti.key'));

    const result = await uploadArcaCredentials(form);

    expect(result.credential?.expiresAt).toBe('2028-01-01T00:00:00Z');
    const request = vi.mocked(apiRequest).mock.calls[0]?.[0];
    expect(request).toMatchObject({ path: '/v1/invoicing/credentials', method: 'PUT' });
    expect((request?.formData?.get('certificate') as File).name).toBe('coti.crt');
    expect((request?.formData?.get('private_key') as File).name).toBe('coti.key');
  });

  it('refuses an upload missing either file', async () => {
    const form = new FormData();
    form.set('certificate', new File(['cert'], 'coti.crt'));

    await expect(uploadArcaCredentials(form)).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('deletes them', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(deleteArcaCredentials()).resolves.toEqual({ ok: true });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/invoicing/credentials',
      method: 'DELETE',
      branchScoped: false,
    });
  });
});
