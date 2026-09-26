import { act, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProductSearchDialog } from '@/app/(protected)/rfqs/[id]/_components/product-search-dialog';
import type { CatalogProduct } from '@/lib/api/catalog';
import messages from '@/translations/es.json';

vi.mock('@/lib/api/catalog', () => ({ searchCatalog: vi.fn() }));

const { searchCatalog } = await import('@/lib/api/catalog');

const CEMENTO_50: CatalogProduct = {
  id: 'p1',
  code: 'CEM-50',
  name: 'Cemento Loma Negra 50kg',
  unit: 'bolsa',
};
const CEMENTO_25: CatalogProduct = {
  id: 'p2',
  code: 'CEM-25',
  name: 'Cemento Loma Negra 25kg',
  unit: 'bolsa',
};
const CAL: CatalogProduct = { id: 'p3', code: 'CAL-25', name: 'Cal hidratada 25kg', unit: 'bolsa' };

function renderDialog(suggestions: CatalogProduct[]) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ProductSearchDialog
        open
        onOpenChange={() => {}}
        onSelect={() => {}}
        suggestions={suggestions}
      />
    </NextIntlClientProvider>,
  );
}

async function settle() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(200);
  });
}

function listedNames(): string[] {
  return screen
    .getAllByRole('button', { name: /Seleccionar/ })
    .map((row) => row.querySelector('p')?.textContent ?? '');
}

describe('ProductSearchDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(searchCatalog).mockResolvedValue([CAL, CEMENTO_50]);
  });

  it('lists the line’s suggestions first, once each, before the catalog', async () => {
    renderDialog([CEMENTO_50, CEMENTO_25]);
    await settle();

    expect(listedNames()).toEqual([CEMENTO_50.name, CEMENTO_25.name, CAL.name]);
    expect(screen.getAllByText(messages.rfqs.detail.items.suggested)).toHaveLength(2);
  });

  it('keeps only the suggestions the search still fits, accents aside', async () => {
    vi.mocked(searchCatalog).mockResolvedValue([]);
    renderDialog([CEMENTO_50, CAL]);
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'cál' } });
    await settle();

    expect(listedNames()).toEqual([CAL.name]);
  });
});
