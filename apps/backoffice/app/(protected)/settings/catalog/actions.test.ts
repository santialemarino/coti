import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createProduct,
  deactivateProduct,
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
  price: '',
  minPrice: '',
};

beforeEach(() => vi.clearAllMocks());

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
        price: null,
        min_price: null,
      },
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
});
