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
    expect(view.getByLabelText(t.taxId.label)).toHaveProperty('value', '30701234568');
    expect(view.getByLabelText(t.address.label)).toHaveProperty('value', 'San Martin 123');
  });
  it('saves to the client and refreshes the profile after updating fiscal details', async () => {
    vi.mocked(updateClientFiscal).mockResolvedValue({
      ...client,
      legalName: 'Obra SA',
      taxId: '30701234568',
      address: 'San Martin 123',
    });
    const view = show();
    fireEvent.change(view.getByLabelText(t.legalName.label), { target: { value: 'Obra SA' } });
    fireEvent.change(view.getByLabelText(t.taxId.label), { target: { value: '30-70123456-8' } });
    fireEvent.change(view.getByLabelText(t.address.label), { target: { value: 'San Martin 123' } });
    fireEvent.submit(view.getByRole('form', { name: t.title }));
    await waitFor(() =>
      expect(updateClientFiscal).toHaveBeenCalledWith(client.id, {
        legalName: 'Obra SA',
        taxId: '30-70123456-8',
        address: 'San Martin 123',
        ivaCondition: 'REGISTERED',
      }),
    );
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(view.getByLabelText(t.taxId.label)).toHaveProperty('value', '30701234568');
  });
  it('prevents an invalid CUIT from reaching the client update', async () => {
    const view = show();
    fireEvent.change(view.getByLabelText(t.taxId.label), { target: { value: '20-12345678-5' } });
    fireEvent.submit(view.getByRole('form', { name: t.title }));
    await waitFor(() => expect(view.getByText(t.taxId.checkDigit)).toBeTruthy());
    expect(updateClientFiscal).not.toHaveBeenCalled();
  });
});
