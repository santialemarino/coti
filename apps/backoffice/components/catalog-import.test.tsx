import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import {
  confirmCatalogImport,
  type CatalogImportPreview,
} from '@/app/(protected)/_actions/catalog-import';
import { CatalogReview } from '@/components/catalog-import';
import messages from '@/translations/es.json';

vi.mock('@/lib/config', () => ({ CATALOG_PREVIEW_PAGE_SIZE: 2 }));
vi.mock('@/app/(protected)/_actions/catalog-import', () => ({ confirmCatalogImport: vi.fn() }));

const preview: CatalogImportPreview = {
  branchId: 'branch',
  validRows: 3,
  invalidRows: 0,
  canConfirm: true,
  previewedAt: '2026-11-09T00:00:00Z',
  rows: ['First product', 'Second product', 'Last product'].map((name, index) => ({
    rowNumber: index + 2,
    code: `P-${index}`,
    name,
    description: '',
    unit: 'unit',
    family: 'Family',
    subgroup: null,
    price: '100.00',
    minPrice: null,
    isActive: true,
    economyAlternative: null,
    premiumAlternative: null,
    action: 'CREATE',
    errors: [],
  })),
};

beforeEach(() => vi.clearAllMocks());

function review() {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview preview={preview} onBack={() => {}} onConfirmed={() => {}} />
    </NextIntlClientProvider>,
  );
}

it('reveals the remaining rows on request', () => {
  const view = review();
  expect(view.queryByText('Last product')).toBeNull();
  fireEvent.click(view.getByRole('button', { name: /Ver más/ }));
  expect(view.getByText('Last product')).toBeTruthy();
  expect(view.queryByRole('button', { name: /Ver más/ })).toBeNull();
});

it('confirms every row even when some have not been revealed', async () => {
  vi.mocked(confirmCatalogImport).mockResolvedValue({
    ok: true,
    createdRows: 3,
    updatedRows: 0,
    skippedRows: 0,
  });
  const view = review();
  fireEvent.click(view.getByRole('button', { name: 'Aplicar 3 cambios' }));
  await waitFor(() => expect(confirmCatalogImport).toHaveBeenCalledWith(preview));
});

it('lists rows with errors before the valid ones', () => {
  const [first, second, last] = preview.rows;
  if (!first || !second || !last) throw new Error('fixture has three rows');
  const failing = { ...last, errors: ['El precio no es válido.'] };
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview
        preview={{
          ...preview,
          validRows: 2,
          invalidRows: 1,
          rows: [first, second, failing],
        }}
        onBack={() => {}}
        onConfirmed={() => {}}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByText('Last product')).toBeTruthy();
  expect(view.queryByText('Second product')).toBeNull();
});

it('shows what each row does to the recommendation slots only when some row changes one', () => {
  const [first, second] = preview.rows;
  if (!first || !second) throw new Error('fixture has rows');
  const plain = review();
  expect(plain.queryByText(messages.catalogImport.table.alternatives)).toBeNull();
  plain.unmount();

  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview
        preview={{
          ...preview,
          rows: [{ ...first, economyAlternative: 'ARE-002', premiumAlternative: '' }, second],
        }}
        onBack={() => {}}
        onConfirmed={() => {}}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByText(messages.catalogImport.table.alternatives)).toBeTruthy();
  expect(view.getByText('Económica: ARE-002')).toBeTruthy();
  expect(view.getByText(messages.catalogImport.alternatives.premiumCleared)).toBeTruthy();
});
