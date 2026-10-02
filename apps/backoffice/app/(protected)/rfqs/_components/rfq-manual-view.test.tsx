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

function view(isAdmin: boolean, branchId = 'b1') {
  return (
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqListProvider
        records={[]}
        activeBranchId={branchId}
        userName="Vendedor Dev"
        userId="u-self"
        isAdmin={isAdmin}
      >
        <RfqManualView
          onBack={vi.fn()}
          onClose={vi.fn()}
          onCreated={vi.fn()}
          activeBranchId={branchId}
          onDirtyChange={vi.fn()}
        />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function renderView(isAdmin: boolean) {
  return render(view(isAdmin));
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
    expect(listSellers).not.toHaveBeenCalled();
  });

  // One seller is still two outcomes — that seller or nobody — so the choice stays, preselected.
  it('preselects the only seller of the branch and keeps the choice open', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA]);
    renderView(true);

    const picker = await screen.findByRole('combobox', { name: 'Vendedor' });
    expect(picker.textContent).toBe(ANA.name);
    expect(screen.getByText('Es el único vendedor de esta sucursal.')).toBeTruthy();
    expect((await createWithOneLine())?.seller_id).toBe(ANA.id);
  });

  it('leaves the order unassigned when the branch has several sellers', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA, BRUNO]);
    renderView(true);

    const picker = await screen.findByRole('combobox', { name: 'Vendedor' });
    expect(picker.textContent).toBe('Sin asignar');
    expect((await createWithOneLine())?.seller_id).toBeNull();
  });

  // A seller of one branch is not a seller of the next.
  it('drops the chosen seller when the branch changes', async () => {
    vi.mocked(listSellers).mockResolvedValue([ANA, BRUNO]);
    const { rerender } = renderView(true);
    fireEvent.click(await screen.findByRole('combobox', { name: 'Vendedor' }));
    fireEvent.click(await screen.findByRole('option', { name: BRUNO.name }));
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Vendedor' }).textContent).toBe(BRUNO.name),
    );

    rerender(view(true, 'b2'));

    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Vendedor' }).textContent).toBe('Sin asignar'),
    );
  });

  it('explains a branch with no seller instead of offering an empty choice', async () => {
    vi.mocked(listSellers).mockResolvedValue([]);
    renderView(true);

    expect(await screen.findByText('Esta sucursal no tiene vendedores')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Sumar un vendedor' })).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: 'Vendedor' })).toBeNull();
    expect((await createWithOneLine())?.seller_id).toBeNull();
  });

  // A failed load is not a branch with no sellers: neither the empty choice nor the setup notice.
  it('says the sellers could not be loaded rather than that there are none', async () => {
    vi.mocked(listSellers).mockRejectedValue(new Error('GET /api/users answered 500'));
    renderView(true);

    expect(await screen.findByText(/No pudimos cargar los vendedores/)).toBeTruthy();
    expect(screen.queryByText('Esta sucursal no tiene vendedores')).toBeNull();
    expect((await createWithOneLine())?.seller_id).toBeNull();
  });
});
