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

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(updateInvoicingSettings).mockResolvedValue({ settings: SETTINGS });
});
describe('InvoicingSettingsForm', () => {
  it('preserves verified points of sale and routes changes through ARCA setup', () => {
    const view = renderForm();
    expect(view.getByText('30-71111111-2')).toBeTruthy();
    expect(view.queryByLabelText('Punto de venta de Centro')).toBeNull();
    expect(view.getByText('Centro')).toBeTruthy();
  });
  it('saves the issuer profile with fiscal settings', async () => {
    const view = renderForm();
    fireEvent.change(view.getByLabelText(copy.profile.address, { exact: false }), {
      target: { value: 'Av. Siempre Viva 123' },
    });
    fireEvent.change(view.getByLabelText(copy.profile.grossIncomeRegistration, { exact: false }), {
      target: { value: 'Exento' },
    });
    fireEvent.change(view.getByLabelText(copy.profile.activityStartedOn, { exact: false }), {
      target: { value: '2020-01-01' },
    });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));
    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          address: 'Av. Siempre Viva 123',
          grossIncomeRegistration: 'Exento',
          activityStartedOn: '2020-01-01',
        }),
      ),
    );
    expect(toast.success).toHaveBeenCalled();
  });
});
