import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProductManager } from '@/app/(protected)/settings/catalog/_components/product-manager';
import type { Product, ProductFamily } from '@/lib/api/products';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  unstable_rethrow: vi.fn(),
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
// The scroll's frames are motion's business; what the shortcut decides is where, how, and then focus.
vi.mock('motion/react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('motion/react')>()),
  useReducedMotion: vi.fn(() => false),
  animate: vi.fn(
    (
      _from: number,
      to: number,
      options: { onUpdate?: (y: number) => void; onComplete?: () => void },
    ) => {
      options.onUpdate?.(to);
      options.onComplete?.();
    },
  ),
}));

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

function renderManager(products: Product[] = [PRODUCT], query = '', bulkEditTargetId?: string) {
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
        bulkEditTargetId={bulkEditTargetId}
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

describe('ProductManager bulk-edit shortcut', () => {
  it('is not offered while there is no bulk-edit section to reach', () => {
    const view = renderManager();

    expect(view.queryByRole('button', { name: messages.products.bulkEdit })).toBeNull();
  });

  it('eases the page to the bulk-edit section and hands focus to its heading', async () => {
    const { animate } = await import('motion/react');
    const { EASE, MOTION } = await import('@repo/ui/lib');
    const section = document.createElement('section');
    section.id = 'catalog-bulk-edit';
    section.style.scrollMarginTop = '80px';
    section.innerHTML = '<h2 tabindex="-1">Edición masiva</h2>';
    document.body.append(section);
    section.getBoundingClientRect = () => ({ top: 900 }) as DOMRect;
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    const height = vi.spyOn(document.documentElement, 'scrollHeight', 'get').mockReturnValue(5000);

    const view = renderManager([PRODUCT], '', 'catalog-bulk-edit');
    fireEvent.click(view.getByRole('button', { name: messages.products.bulkEdit }));

    expect(vi.mocked(animate)).toHaveBeenCalledWith(
      0,
      820,
      expect.objectContaining({ duration: MOTION.slower, ease: EASE.inOutSoft }),
    );
    expect(scrollTo).toHaveBeenCalledWith(0, 820);
    expect(document.activeElement?.textContent).toBe('Edición masiva');
    section.remove();
    scrollTo.mockRestore();
    height.mockRestore();
  });

  // A section near the end of the page cannot reach the top; the travel stops where the page does.
  it('never aims past the last scroll position the page has', async () => {
    const { animate } = await import('motion/react');
    const section = document.createElement('section');
    section.id = 'catalog-bulk-edit';
    document.body.append(section);
    section.getBoundingClientRect = () => ({ top: 900 }) as DOMRect;
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    vi.spyOn(document.documentElement, 'scrollHeight', 'get').mockReturnValue(1268);

    const view = renderManager([PRODUCT], '', 'catalog-bulk-edit');
    fireEvent.click(view.getByRole('button', { name: messages.products.bulkEdit }));

    expect(vi.mocked(animate).mock.lastCall?.[1]).toBe(1268 - window.innerHeight);
    section.remove();
    vi.restoreAllMocks();
  });

  it('jumps without easing for a caller who asked for less motion', async () => {
    const { animate, useReducedMotion } = await import('motion/react');
    vi.mocked(useReducedMotion).mockReturnValue(true);
    const section = document.createElement('section');
    section.id = 'catalog-bulk-edit';
    document.body.append(section);
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});

    const view = renderManager([PRODUCT], '', 'catalog-bulk-edit');
    fireEvent.click(view.getByRole('button', { name: messages.products.bulkEdit }));

    expect(vi.mocked(animate).mock.lastCall?.[2]).toEqual(expect.objectContaining({ duration: 0 }));
    section.remove();
    vi.mocked(useReducedMotion).mockReturnValue(false);
  });
});
