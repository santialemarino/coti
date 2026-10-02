import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqManualView } from '@/app/(protected)/rfqs/_components/rfq-manual-view';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/catalog', () => ({
  searchCatalog: vi.fn(async () => [
    { id: 'p1', name: 'Cemento 50 kg', code: 'CEM-50', unit: 'bolsa', price: '9200.00' },
  ]),
}));
vi.mock('@/lib/api/sellers', () => ({ listSellers: vi.fn() }));
vi.mock('@/lib/api/rfqs-client', () => ({ createRfq: vi.fn(async () => ({ id: 'r1' })) }));

const { listSellers } = await import('@/lib/api/sellers');
const { createRfq } = await import('@/lib/api/rfqs-client');

const ANA = { id: 'u-ana', name: 'Ana Gómez' };
const BRUNO = { id: 'u-bruno', name: 'Bruno Díaz' };

function renderView(isAdmin: boolean) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqListProvider
        records={[]}
        activeBranchId="b1"
        userName="Vendedor Dev"
        userId="u-self"
        isAdmin={isAdmin}
      >
        <RfqManualView
          onBack={vi.fn()}
          onClose={vi.fn()}
          onCreated={vi.fn()}
          activeBranchId="b1"
          onDirtyChange={vi.fn()}
        />
      </RfqListProvider>
    </NextIntlClientProvider>,
  );
}

async function createWithOneLine() {
  fireEvent.click(await screen.findByRole('button', { name: /Agregar/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Crear pedido' }));
  await waitFor(() => expect(createRfq).toHaveBeenCalled());
  return vi.mocked(createRfq).mock.calls[0]?.[0];
}

describe('RfqManualView seller', () => {
  beforeEach(() => vi.clearAllMocks());

  it('gives a seller their own order, with nothing to pick', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA, BRUNO]);
    renderView(false);

    expect(await screen.findByText('El pedido queda a tu nombre.')).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: 'Vendedor' })).toBeNull();
    expect((await createWithOneLine())?.seller_id).toBe('u-self');
  });

  it('settles on the only seller of the branch for an administrator', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA]);
    renderView(true);

    expect(await screen.findByText('Es el único vendedor de esta sucursal.')).toBeTruthy();
    expect(screen.getByText(ANA.name)).toBeTruthy();
    expect((await createWithOneLine())?.seller_id).toBe(ANA.id);
  });

  it('lets an administrator choose when the branch has several sellers', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA, BRUNO]);
    renderView(true);

    expect(await screen.findByRole('combobox', { name: 'Vendedor' })).toBeTruthy();
    expect((await createWithOneLine())?.seller_id).toBeNull();
  });
});
