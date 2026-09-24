import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProductManager } from '@/app/(protected)/settings/catalog/_components/product-manager';
import type { Product, ProductFamily } from '@/lib/api/products';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/settings/catalog',
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock('@/app/(protected)/settings/catalog/actions', () => ({
  createProduct: vi.fn(),
  updateProduct: vi.fn(),
  deactivateProduct: vi.fn(),
  reactivateProduct: vi.fn(),
  uploadProductImage: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const { deactivateProduct, reactivateProduct } =
  await import('@/app/(protected)/settings/catalog/actions');

const FAMILY: ProductFamily = {
  id: 'family-1',
  name: 'MATERIALES DE CONSTRUCCION',
  subgroups: [{ id: 'subgroup-1', name: 'CEMENTOS' }],
};
const PRODUCT: Product = {
  id: 'product-1',
  code: 'CEM-001',
  name: 'Cemento Portland',
  description: 'Bolsa de 50 kg',
  unit: 'bolsa',
  familyId: FAMILY.id,
  subgroupId: FAMILY.subgroups[0]?.id ?? null,
  imageUrl: null,
  isActive: true,
  createdAt: '2026-09-01T12:00:00Z',
  updatedAt: '2026-09-01T12:00:00Z',
};

function renderManager(products: Product[] = [PRODUCT], query = '') {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ProductManager
        page={{ items: products, total: products.length, limit: 20, offset: 0 }}
        families={[FAMILY]}
        query={query}
      />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('ProductManager', () => {
  it('lists catalog fields with the existing table and status components', () => {
    const view = renderManager();
    const row = within(view.getByRole('row', { name: new RegExp(PRODUCT.name) }));

    expect(row.getByText(PRODUCT.code ?? '')).toBeTruthy();
    expect(row.getByText(FAMILY.name)).toBeTruthy();
    expect(row.getByText(messages.products.status.active)).toBeTruthy();
  });

  it('opens the shared form dialog with the selected product values', async () => {
    const view = renderManager();
    const row = within(view.getByRole('row', { name: new RegExp(PRODUCT.name) }));

    fireEvent.click(row.getByRole('button', { name: messages.products.edit.action }));

    const dialog = await view.findByRole('dialog');
    const name = dialog.querySelector('input[name="name"]');
    const code = dialog.querySelector('input[name="code"]');
    await waitFor(() => expect(name).toHaveProperty('value', PRODUCT.name));
    expect(code).toHaveProperty('value', PRODUCT.code);
  });

  it('confirms before soft-deactivating a product', async () => {
    vi.mocked(deactivateProduct).mockResolvedValue({ ok: true });
    const view = renderManager();
    const row = within(view.getByRole('row', { name: new RegExp(PRODUCT.name) }));

    fireEvent.click(row.getByRole('button', { name: messages.products.deactivate.action }));
    const dialog = await view.findByRole('dialog');
    fireEvent.click(
      within(dialog).getByRole('button', { name: messages.products.deactivate.confirm }),
    );

    await waitFor(() => expect(deactivateProduct).toHaveBeenCalledWith(PRODUCT.id));
  });

  it('reactivates an inactive product directly with its current attributes', async () => {
    vi.mocked(reactivateProduct).mockResolvedValue({ ok: true, productId: PRODUCT.id });
    const inactive = { ...PRODUCT, isActive: false };
    const view = renderManager([inactive]);
    const row = within(view.getByRole('row', { name: new RegExp(PRODUCT.name) }));

    fireEvent.click(row.getByRole('button', { name: messages.products.reactivate.action }));

    await waitFor(() => expect(reactivateProduct).toHaveBeenCalledOnce());
    expect(reactivateProduct).toHaveBeenCalledWith(
      PRODUCT.id,
      expect.objectContaining({ name: PRODUCT.name, isActive: true }),
    );
  });
});
