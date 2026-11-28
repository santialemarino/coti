import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Branch } from '@/lib/api/branches';

vi.mock('next-intl/server', () => ({ getTranslations: async () => (key: string) => key }));
vi.mock('next/navigation', () => ({
  notFound: () => {
    throw new Error('NOT_FOUND');
  },
}));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getEffectiveBranchId: vi.fn(async () => 'one') }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/api/account', () => ({ getAccount: async () => ({ invoicingEnabled: false }) }));
vi.mock('@/lib/api/products', () => ({
  getProducts: async () => ({ items: [], total: 0, limit: 20, offset: 0 }),
  getProductTaxonomy: async () => [],
}));
vi.mock('@/app/(protected)/_components/page-header', () => ({ PageHeader: () => null }));
vi.mock('@/app/(protected)/settings/prices/_components/price-import', () => ({
  PriceImport: vi.fn(() => null),
}));
vi.mock('@/components/catalog-import', () => ({ CatalogImport: vi.fn(() => null) }));
vi.mock('@/app/(protected)/settings/catalog/_components/product-manager', () => ({
  ProductManager: () => null,
}));

const { getBranches } = await import('@/lib/api/branches');
const { getEffectiveBranchId } = await import('@/lib/auth/branch');
const { default: PriceSettingsPage } = await import('@/app/(protected)/settings/prices/page');
const { default: CatalogSettingsPage } = await import('@/app/(protected)/settings/catalog/page');
const { PriceImport } = await import('@/app/(protected)/settings/prices/_components/price-import');
const { CatalogImport } = await import('@/components/catalog-import');

const branches: Branch[] = ['one', 'two'].map((id) => ({
  id,
  name: id,
  address: null,
  email: null,
  defaultExpiryDays: 7,
  isActive: true,
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getBranches).mockResolvedValue(branches);
});

it('keeps the alerted branch when opening the catalog shortcut from prices', async () => {
  render(await PriceSettingsPage({ searchParams: Promise.resolve({ branch_id: 'two' }) }));
  expect(screen.getByRole('link', { name: 'individualAction' }).getAttribute('href')).toBe(
    '/settings/catalog?branch_id=two',
  );
});

it('opens the account catalog when no price branch is selected', async () => {
  vi.mocked(getEffectiveBranchId).mockResolvedValueOnce(undefined);
  render(await PriceSettingsPage({ searchParams: Promise.resolve({}) }));
  expect(screen.getByRole('link', { name: 'individualAction' }).getAttribute('href')).toBe(
    '/settings/catalog',
  );
});

describe.each([
  { name: 'prices', page: PriceSettingsPage, importer: PriceImport },
  { name: 'catalog', page: CatalogSettingsPage, importer: CatalogImport },
])('readiness navigation to $name', ({ page, importer }) => {
  it('uses the alerted branch even when a different branch is selected', async () => {
    render(await page({ searchParams: Promise.resolve({ branch_id: 'two' }) }));
    expect(importer).toHaveBeenCalledWith(
      expect.objectContaining({ branch: branches[1] }),
      undefined,
    );
    expect(getEffectiveBranchId).not.toHaveBeenCalled();
  });

  it('preserves the selected branch when no branch is requested', async () => {
    render(await page({ searchParams: Promise.resolve({}) }));
    expect(importer).toHaveBeenCalledWith(
      expect.objectContaining({ branch: branches[0] }),
      undefined,
    );
  });

  it.each(['foreign-or-inactive', '', ['one', 'two']])(
    'refuses an unavailable branch %j',
    async (branchId) => {
      await expect(
        page({ searchParams: Promise.resolve({ branch_id: branchId }) }),
      ).rejects.toThrow('NOT_FOUND');
      expect(importer).not.toHaveBeenCalled();
    },
  );
});
