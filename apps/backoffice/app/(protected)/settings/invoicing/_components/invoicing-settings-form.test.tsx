import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { updateInvoicingSettings } from '@/app/(protected)/settings/invoicing/actions';
import type { InvoicingSettings } from '@/lib/api/invoicing';
import messages from '@/translations/es.json';
import { InvoicingSettingsForm } from './invoicing-settings-form';

vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/app/(protected)/settings/invoicing/actions', () => ({
  updateInvoicingSettings: vi.fn(),
}));

const copy = messages.invoicing.settings;

const SETTINGS: InvoicingSettings = {
  enabled: true,
  homologation: true,
  legalName: 'Corralón del Sur SRL',
  taxId: '30711111112',
  ivaCondition: 'REGISTERED',
  pricesIncludeVat: true,
  branches: [
    { branchId: 'b1', name: 'Centro', isActive: true, pointOfSale: 3 },
    { branchId: 'b2', name: 'Morón', isActive: false, pointOfSale: null },
  ],
  credential: null,
};

function renderForm() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <InvoicingSettingsForm settings={SETTINGS} />
    </NextIntlClientProvider>,
  );
}

function pointOfSale(view: ReturnType<typeof renderForm>, branch: string) {
  return view.getByLabelText(`Punto de venta de ${branch}`);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(updateInvoicingSettings).mockResolvedValue({ settings: SETTINGS });
});

describe('InvoicingSettingsForm', () => {
  it('shows the account’s fiscal identity read-only, with the CUIT written out', () => {
    const view = renderForm();

    expect(view.getByText('Corralón del Sur SRL')).toBeTruthy();
    expect(view.getByText('30-71111111-2')).toBeTruthy();
    expect(view.getByText(copy.pointsOfSale.closed)).toBeTruthy();
  });

  it('saves every branch’s point of sale and names what changed', async () => {
    const view = renderForm();

    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '4' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith({
        ivaCondition: 'REGISTERED',
        pricesIncludeVat: true,
        branches: [
          { branchId: 'b1', pointOfSale: '3' },
          { branchId: 'b2', pointOfSale: '4' },
        ],
      }),
    );
    expect(toast.success).toHaveBeenCalledWith('Guardamos el punto de venta de Morón.');
  });

  it('refuses a point of sale another branch already uses, on its own row', async () => {
    const view = renderForm();

    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '3' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    expect(await view.findByText(copy.pointOfSale.duplicate)).toBeTruthy();
    expect(updateInvoicingSettings).not.toHaveBeenCalled();
  });
});
