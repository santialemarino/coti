import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  confirmCatalogImport,
  type CatalogImportPreview,
} from '@/app/(protected)/_actions/catalog-import';

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

describe('confirmCatalogImport', () => {
  it('submits reviewed rows and keeps preview rejections in the result', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ imported_rows: 2, skipped_rows: 0 });
    const preview: CatalogImportPreview = {
      branchId: BRANCH_ID,
      rows: [
        row({ rowNumber: 2, code: 'CEM-001', name: 'Cemento' }),
        row({ rowNumber: 3, code: 'ARE-001', name: 'Arena' }),
        row({
          rowNumber: 4,
          code: 'CAL-001',
          name: 'Cal',
          subgroup: null,
          errors: ['invalid_subgroup'],
        }),
      ],
      validRows: 2,
      invalidRows: 1,
      canConfirm: true,
      previewedAt: '2026-08-18T12:00:00Z',
    };

    await expect(confirmCatalogImport(preview)).resolves.toEqual({
      ok: true,
      importedRows: 2,
      skippedRows: 1,
    });
    expect(requestSent()?.body).toEqual({
      rows: [
        {
          code: 'CEM-001',
          name: 'Cemento',
          description: '',
          unit: 'unidad',
          family: 'MATERIALES DE CONSTRUCCION',
          subgroup: null,
          price: '100.00',
          min_price: null,
        },
        {
          code: 'ARE-001',
          name: 'Arena',
          description: '',
          unit: 'unidad',
          family: 'MATERIALES DE CONSTRUCCION',
          subgroup: null,
          price: '100.00',
          min_price: null,
        },
      ],
    });
  });

  it('adds rows rejected by backend revalidation to preview rejections', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ imported_rows: 1, skipped_rows: 1 });
    const preview: CatalogImportPreview = {
      branchId: BRANCH_ID,
      rows: [
        row({ rowNumber: 2, code: 'CEM-001', name: 'Cemento' }),
        row({ rowNumber: 3, code: 'ARE-001', name: 'Arena' }),
        row({ rowNumber: 4, code: 'CAL-001', name: 'Cal', errors: ['invalid_min_price'] }),
      ],
      validRows: 2,
      invalidRows: 1,
      canConfirm: true,
      previewedAt: '2026-08-18T12:00:00Z',
    };

    await expect(confirmCatalogImport(preview)).resolves.toEqual({
      ok: true,
      importedRows: 1,
      skippedRows: 2,
    });
  });
});

function row(overrides: Partial<CatalogImportPreview['rows'][number]>) {
  return {
    rowNumber: 2,
    code: 'CODE',
    name: 'Product',
    description: '',
    unit: 'unidad',
    family: 'MATERIALES DE CONSTRUCCION',
    subgroup: null,
    price: '100.00',
    minPrice: null,
    errors: [],
    ...overrides,
  };
}
