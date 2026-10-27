import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ClientFiscalForm } from '@/app/(protected)/clients/[id]/_components/client-fiscal-form';
import { updateClientFiscal } from '@/lib/api/clients-client';
import type { ClientFiscal } from '@/lib/api/invoicing';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({ updateClientFiscal: vi.fn() }));
const client: ClientFiscal = {
  id: 'client-1',
  name: 'Eduardo',
  legalName: null,
  taxId: null,
  dni: null,
  address: null,
  ivaCondition: 'REGISTERED',
};
const t = messages.clients.fiscal;
function show(data: ClientFiscal = client) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientFiscalForm client={data} />
    </NextIntlClientProvider>,
  );
}
beforeEach(() => vi.clearAllMocks());
describe('client profile fiscal data', () => {
  it('is available without a quote or invoice and loads the client saved values', () => {
    const view = show({
      ...client,
      legalName: 'Obra SA',
      taxId: '30701234568',
      address: 'San Martin 123',
    });
    expect(view.getByLabelText(t.legalName.label)).toHaveProperty('value', 'Obra SA');
    expect(view.getByLabelText(t.address.label)).toHaveProperty('value', 'San Martin 123');
  });

  // Only the customer loads a CUIT, by creating their account; the seller reads it.
  it('shows the CUIT read-only, with who loads it, and offers no field for it', () => {
    const view = show({ ...client, taxId: '30701234568', dni: '28123456' });

    expect(view.getByText('30-70123456-8')).toBeTruthy();
    expect(view.getByText(t.cuit.hint)).toBeTruthy();
    expect(
      view.getAllByRole('textbox').map((box) => (box as HTMLInputElement).value),
    ).not.toContain('30-70123456-8');
    // The DNI is its own field, loaded from its own value, never from the CUIT.
    expect(view.getByLabelText(t.dni.label)).toHaveProperty('value', '28.123.456');
  });

  it('says the CUIT is not loaded yet and still takes a DNI', () => {
    const view = show();

    expect(view.getByText(t.cuit.missing)).toBeTruthy();
    expect(view.getByText(t.cuit.hint)).toBeTruthy();
    expect(view.getByLabelText(t.dni.label)).toBeTruthy();
  });

  it('saves to the client without a CUIT and refreshes the profile', async () => {
    vi.mocked(updateClientFiscal).mockResolvedValue({
      ...client,
      legalName: 'Obra SA',
      taxId: '30701234568',
      address: 'San Martin 123',
    });
    const view = show({ ...client, taxId: '30701234568' });
    fireEvent.change(view.getByLabelText(t.legalName.label), { target: { value: 'Obra SA' } });
    fireEvent.change(view.getByLabelText(t.address.label), { target: { value: 'San Martin 123' } });
    fireEvent.submit(view.getByRole('form', { name: t.title }));
    await waitFor(() =>
      expect(updateClientFiscal).toHaveBeenCalledWith(client.id, {
        legalName: 'Obra SA',
        dni: undefined,
        address: 'San Martin 123',
        ivaCondition: 'REGISTERED',
      }),
    );
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
  });

  it('takes a DNI in its own shape and saves its digits', async () => {
    vi.mocked(updateClientFiscal).mockResolvedValue({ ...client, taxId: '12345678' });
    const view = show();
    const dni = view.getByLabelText(t.dni.label);

    fireEvent.change(dni, { target: { value: '12 345 678' } });
    expect(dni).toHaveProperty('value', '12.345.678');
    fireEvent.submit(view.getByRole('form', { name: t.title }));

    await waitFor(() =>
      expect(updateClientFiscal).toHaveBeenCalledWith(
        client.id,
        expect.objectContaining({ dni: '12345678' }),
      ),
    );
  });

  it('refuses a DNI of the wrong length', async () => {
    const view = show();

    fireEvent.change(view.getByLabelText(t.dni.label), { target: { value: '123456' } });
    fireEvent.submit(view.getByRole('form', { name: t.title }));

    await waitFor(() => expect(view.getByText(t.dni.invalid)).toBeTruthy());
    expect(updateClientFiscal).not.toHaveBeenCalled();
  });
});
