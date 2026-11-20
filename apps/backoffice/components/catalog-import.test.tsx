import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import {
  confirmCatalogImport,
  type CatalogImportPreview,
} from '@/app/(protected)/_actions/catalog-import';
import { CatalogReview } from '@/components/catalog-import';
import messages from '@/translations/es.json';

vi.mock('@/lib/config', () => ({ IMPORT_PREVIEW_PAGE_SIZE: 2 }));
vi.mock('@/app/(protected)/_actions/catalog-import', () => ({ confirmCatalogImport: vi.fn() }));

const preview: CatalogImportPreview = {
  branchId: 'branch',
  validRows: 3,
  invalidRows: 0,
  createRows: 3,
  updateRows: 0,
  unchangedRows: 0,
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
  fireEvent.click(view.getByRole('button', { name: 'Ver 1 más' }));
  expect(view.getByText('Last product')).toBeTruthy();
  expect(view.queryByRole('button', { name: 'Ver 1 más' })).toBeNull();
});

it('confirms every row even when some have not been revealed', async () => {
  vi.mocked(confirmCatalogImport).mockResolvedValue({
    ok: true,
    createdRows: 3,
    updatedRows: 0,
    unchangedRows: 0,
    skippedRows: 0,
  });
  const view = review();
  fireEvent.click(view.getByRole('button', { name: 'Aplicar 3 cambios' }));
  await waitFor(() => expect(confirmCatalogImport).toHaveBeenCalledWith(preview));
});

it('counts only the rows that change something and lists the unchanged ones last', () => {
  const [first, second, last] = preview.rows;
  if (!first || !second || !last) throw new Error('fixture has three rows');
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview
        preview={{
          ...preview,
          createRows: 0,
          updateRows: 1,
          unchangedRows: 2,
          rows: [
            { ...first, action: 'UNCHANGED' },
            { ...second, action: 'UNCHANGED' },
            { ...last, action: 'UPDATE' },
          ],
        }}
        onBack={() => {}}
        onConfirmed={() => {}}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByRole('button', { name: 'Aplicar 1 cambio' })).toBeTruthy();
  // The page shows two rows: the one change first, then the first unchanged one.
  const [changed, unchanged] = view.getAllByRole('row').slice(1);
  expect(changed?.textContent).toContain('Last product');
  expect(changed?.textContent).toContain(messages.catalogImport.action.UPDATE);
  expect(unchanged?.textContent).toContain('First product');
  expect(unchanged?.textContent).toContain(messages.catalogImport.action.UNCHANGED);
  expect(view.queryByText('Second product')).toBeNull();
});

it('says there is nothing to apply when every valid row matches the catalog', () => {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview
        preview={{
          ...preview,
          createRows: 0,
          unchangedRows: 3,
          canConfirm: false,
          rows: preview.rows.map((row) => ({ ...row, action: 'UNCHANGED' as const })),
        }}
        onBack={() => {}}
        onConfirmed={() => {}}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByText(messages.catalogImport.noChanges)).toBeTruthy();
  expect(view.queryByRole('button', { name: /Aplicar/ })).toBeNull();
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
          createRows: 2,
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

it('reveals every row at once on request', () => {
  const base = preview.rows[0];
  if (!base) throw new Error('fixture has rows');
  const rows = Array.from({ length: 6 }, (_, index) => ({
    ...base,
    rowNumber: index + 2,
    code: `R-${index}`,
    name: `Row ${index + 1}`,
  }));
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CatalogReview
        preview={{ ...preview, validRows: 6, createRows: 6, rows }}
        onBack={() => {}}
        onConfirmed={() => {}}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByRole('status').textContent).toBe('Mostrando 2 de 6 filas');
  fireEvent.click(view.getByRole('button', { name: 'Ver todas (6)' }));
  expect(view.getByText('Row 6')).toBeTruthy();
  expect(document.activeElement).toBe(view.getByText('Row 3').closest('tr'));
});
