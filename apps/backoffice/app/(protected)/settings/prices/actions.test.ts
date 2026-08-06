import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  confirmPriceImport,
  previewPriceImport,
  type ProductPriceImportPreview,
} from '@/app/(protected)/settings/prices/actions';

vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

const BRANCH_ID = '11111111-1111-4111-8111-111111111111';

function requestSent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0];
}

beforeEach(() => vi.clearAllMocks());

describe('previewPriceImport', () => {
  it('maps the backend-derived currency without spreadsheet conditions', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      rows: [
        {
          row_number: 2,
          code: 'CEM-001',
          product_name: 'Cemento',
          current_price: '9500.00',
          current_min_price: '9000.00',
          price: '10000.00',
          min_price: '9200.00',
          currency: 'USD',
          errors: [],
        },
      ],
      valid_rows: 1,
      invalid_rows: 0,
      can_confirm: true,
      previewed_at: '2026-08-05T12:00:00Z',
    });
    const formData = new FormData();
    formData.set('file', new File(['codigo,precio\nCEM-001,10000'], 'precios.csv'));

    const result = await previewPriceImport(BRANCH_ID, formData);

    expect(result).toEqual({
      ok: true,
      preview: {
        branchId: BRANCH_ID,
        rows: [
          {
            rowNumber: 2,
            code: 'CEM-001',
            productName: 'Cemento',
            currentPrice: '9500.00',
            currentMinPrice: '9000.00',
            price: '10000.00',
            minPrice: '9200.00',
            currency: 'USD',
            errors: [],
          },
        ],
        validRows: 1,
        invalidRows: 0,
        canConfirm: true,
        previewedAt: '2026-08-05T12:00:00Z',
      },
    });
  });

  it('normalizes null row errors from the API', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      rows: [
        {
          row_number: 2,
          code: 'CEM-001',
          product_name: 'Cemento',
          current_price: null,
          current_min_price: null,
          price: '10000.00',
          min_price: null,
          currency: 'ARS',
          errors: null,
        },
      ],
      valid_rows: 1,
      invalid_rows: 0,
      can_confirm: true,
      previewed_at: '2026-08-05T12:00:00Z',
    });
    const formData = new FormData();
    formData.set('file', new File(['codigo,precio\nCEM-001,10000'], 'precios.csv'));

    const result = await previewPriceImport(BRANCH_ID, formData);

    expect(result.ok && result.preview.rows[0]?.errors).toEqual([]);
  });
});

describe('confirmPriceImport', () => {
  it('does not let the client send currency or conditions', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ imported_rows: 1 });
    const preview: ProductPriceImportPreview = {
      branchId: BRANCH_ID,
      rows: [
        {
          rowNumber: 2,
          code: 'CEM-001',
          productName: 'Cemento',
          currentPrice: '9500.00',
          currentMinPrice: '9000.00',
          price: '10000.00',
          minPrice: '9200.00',
          currency: 'USD',
          errors: [],
        },
      ],
      validRows: 1,
      invalidRows: 0,
      canConfirm: true,
      previewedAt: '2026-08-05T12:00:00Z',
    };

    await expect(confirmPriceImport(preview)).resolves.toEqual({ ok: true, importedRows: 1 });
    expect(requestSent()?.body).toEqual({
      rows: [{ code: 'CEM-001', price: '10000.00', min_price: '9200.00' }],
    });
  });
});
