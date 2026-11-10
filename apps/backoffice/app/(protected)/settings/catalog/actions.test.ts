import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createProduct,
  deactivateProduct,
  getProductPrice,
  listProductRecommendations,
  removeProductRecommendation,
  setProductPrice,
  setProductRecommendation,
  updateProduct,
  uploadProductImage,
} from '@/app/(protected)/settings/catalog/actions';
import type { ProductValues } from '@/app/(protected)/settings/catalog/form-schema';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

const VALUES: ProductValues = {
  code: 'CEM-001',
  name: 'Cemento Portland',
  description: 'Bolsa de 50 kg',
  unit: 'bolsa',
  familyId: '11111111-1111-4111-8111-111111111111',
  subgroupId: '',
  isActive: true,
  vatRate: 'VAT_21',
  price: '',
  minPrice: '',
};

beforeEach(() => vi.clearAllMocks());

describe('catalog branch prices', () => {
  const values = {
    branchId: 'b0000000-0000-4000-8000-000000000002',
    price: '5000.25',
    minPrice: '4800',
    currency: 'ARS',
  };

  it('reads the current period for the explicitly selected branch and maps decimal strings', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [
        {
          price: '6000',
          min_price: null,
          currency: 'ARS',
          valid_from: '2099-01-01T00:00:00Z',
          valid_to: null,
        },
        {
          price: '5000.25',
          min_price: '4800.00',
          currency: 'ARS',
          valid_from: '2020-01-01T00:00:00Z',
          valid_to: null,
        },
        {
          price: '4000',
          min_price: null,
          currency: 'ARS',
          valid_from: '2019-01-01T00:00:00Z',
          valid_to: '2020-01-01T00:00:00Z',
        },
      ],
    });
    await expect(getProductPrice('product-1', values.branchId)).resolves.toEqual({
      ok: true,
      current: { price: '5000.25', minPrice: '4800.00', currency: 'ARS' },
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/product-1/prices',
      branchId: values.branchId,
    });
  });

  it('creates a new validity period without converting amounts or inferring the branch', async () => {
    vi.mocked(apiRequest).mockResolvedValue({});
    await expect(setProductPrice('product-1', values)).resolves.toEqual({
      ok: true,
      productId: 'product-1',
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/product-1/prices',
      method: 'POST',
      branchId: values.branchId,
      body: { price: '5000.25', min_price: '4800', currency: 'ARS' },
    });
  });

  it('clears an optional floor explicitly', async () => {
    vi.mocked(apiRequest).mockResolvedValue({});
    await setProductPrice('product-1', { ...values, minPrice: '' });
    expect(apiRequest).toHaveBeenCalledWith(
      expect.objectContaining({ body: { price: '5000.25', min_price: null, currency: 'ARS' } }),
    );
  });

  it.each([
    { branchId: '' },
    { price: '' },
    { price: '-1' },
    { price: '1.234' },
    { price: 'NaN' },
    { minPrice: '5000.26' },
  ])('rejects invalid price input before writing (%j)', async (invalid) => {
    await expect(setProductPrice('product-1', { ...values, ...invalid })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('keeps a failed load distinct from an unpriced product', async () => {
    vi.mocked(apiRequest).mockRejectedValueOnce(new Error('offline'));
    await expect(getProductPrice('product-1', values.branchId)).resolves.toEqual({
      error: 'INTERNAL',
    });
    vi.mocked(apiRequest).mockResolvedValueOnce({ items: [] });
    await expect(getProductPrice('product-1', values.branchId)).resolves.toEqual({
      ok: true,
      current: null,
    });
  });
});

describe('catalog product actions', () => {
  it('maps the create form to the API contract', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'product-1' });

    await expect(createProduct(VALUES)).resolves.toEqual({ ok: true, productId: 'product-1' });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products',
      method: 'POST',
      branchScoped: false,
      body: {
        code: 'CEM-001',
        canonical_name: 'Cemento Portland',
        description: 'Bolsa de 50 kg',
        unit: 'bolsa',
        family_id: VALUES.familyId,
        subgroup_id: null,
        vat_rate: 'VAT_21',
        price: null,
        min_price: null,
      },
    });
  });

  it('sends the product’s VAT rate on create and on edit', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'product-1' });

    await createProduct({ ...VALUES, vatRate: 'VAT_10_5' });
    await updateProduct('product-1', { ...VALUES, vatRate: 'EXEMPT' });

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      body: { vat_rate: 'VAT_10_5' },
    });
    expect(vi.mocked(apiRequest).mock.calls[1]?.[0]).toMatchObject({
      method: 'PUT',
      body: { vat_rate: 'EXEMPT' },
    });
  });

  it('sends an initial price and floor as decimal strings on create', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'product-1' });

    await createProduct({ ...VALUES, price: '12500.5', minPrice: '11000' });

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      body: { price: '12500.5', min_price: '11000' },
    });
  });

  it('refuses a floor above the price before calling the API', async () => {
    await expect(createProduct({ ...VALUES, price: '100', minPrice: '100.01' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    await expect(createProduct({ ...VALUES, minPrice: '100' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('does not call the API with an invalid product', async () => {
    await expect(createProduct({ ...VALUES, name: ' ' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('soft-deletes the product outside branch scope', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(deactivateProduct('product-1')).resolves.toEqual({ ok: true });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/product-1',
      method: 'DELETE',
      branchScoped: false,
    });
  });

  it('uploads the chosen image as multipart data', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);
    const formData = new FormData();
    formData.set('file', new File(['image'], 'product.webp', { type: 'image/webp' }));

    await expect(uploadProductImage('product-1', formData)).resolves.toEqual({ ok: true });
    expect(apiRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        path: '/v1/products/product-1/image',
        method: 'POST',
        branchScoped: false,
        formData: expect.any(FormData),
      }),
    );
  });

  it('reads only the commercial recommendation slots from the catalog relation list', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [
        {
          id: 'economy-link',
          alternative_product_id: 'economy-product',
          type: 'ECONOMY',
          product: { code: 'CEM-ECO', canonical_name: 'Cemento económico', unit: 'bolsa' },
        },
        {
          id: 'equivalent-link',
          alternative_product_id: 'equivalent-product',
          type: 'EQUIVALENT',
          product: { code: 'CEM-EQ', canonical_name: 'Cemento equivalente', unit: 'bolsa' },
        },
      ],
    });

    await expect(listProductRecommendations('base-product')).resolves.toEqual({
      ok: true,
      items: [
        {
          id: 'economy-link',
          productId: 'economy-product',
          type: 'ECONOMY',
          name: 'Cemento económico',
          code: 'CEM-ECO',
          unit: 'bolsa',
        },
      ],
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/base-product/alternatives',
      branchScoped: false,
    });
  });

  it('replaces a catalog recommendation and returns its new relation id', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'premium-link' });

    await expect(
      setProductRecommendation('base-product', 'PREMIUM', 'premium-product'),
    ).resolves.toEqual({ ok: true, productId: 'base-product', recommendationId: 'premium-link' });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/base-product/recommendations/PREMIUM',
      method: 'PUT',
      branchScoped: false,
      body: { alternative_product_id: 'premium-product' },
    });
  });

  it('removes a configured recommendation through its catalog link', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(removeProductRecommendation('base-product', 'economy-link')).resolves.toEqual({
      ok: true,
      productId: 'base-product',
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/products/base-product/alternatives/economy-link',
      method: 'DELETE',
      branchScoped: false,
    });
  });
});
