import { fireEvent, render, waitFor, within } from '@testing-library/react';
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
  profile: {
    address: 'Av. Rivadavia 1234',
    grossIncomeRegistration: '901-123456-7',
    activityStartedOn: '2020-01-02',
  },
  legalName: 'Corralón del Sur SRL',
  taxId: '30711111112',
  ivaCondition: 'REGISTERED',
  pricesIncludeVat: true,
  invoiceOnPayment: false,
  branches: [
    { branchId: 'b1', name: 'Centro', isActive: true, pointOfSale: 3 },
    { branchId: 'b2', name: 'Morón', isActive: false, pointOfSale: null },
  ],
};

function renderForm(settings: InvoicingSettings = SETTINGS) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <InvoicingSettingsForm settings={settings} />
    </NextIntlClientProvider>,
  );
}

function pointOfSale(view: ReturnType<typeof renderForm>, branch: string): HTMLInputElement {
  return view.getByLabelText(`Punto de venta de ${branch}`) as HTMLInputElement;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(updateInvoicingSettings).mockResolvedValue({ settings: SETTINGS });
});

describe('InvoicingSettingsForm points of sale', () => {
  it('edits each branch’s point of sale in place and saves them with the rest', async () => {
    const view = renderForm();
    expect(pointOfSale(view, 'Centro').value).toBe('3');

    fireEvent.change(pointOfSale(view, 'Centro'), { target: { value: '' } });
    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '4' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          branches: [
            { branchId: 'b1', pointOfSale: '' },
            { branchId: 'b2', pointOfSale: '4' },
          ],
        }),
      ),
    );
    expect(toast.success).toHaveBeenCalled();
  });

  // A tab left open on old numbers must not undo what was saved since: unchanged rows stay home.
  it('sends only the rows that changed', async () => {
    const view = renderForm();

    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '4' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith(
        expect.objectContaining({ branches: [{ branchId: 'b2', pointOfSale: '4' }] }),
      ),
    );
  });

  it('refuses one number for two branches on the second row', async () => {
    const view = renderForm();

    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '3' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() => expect(view.getByText(copy.pointOfSale.duplicate)).toBeTruthy());
    expect(pointOfSale(view, 'Morón').getAttribute('aria-invalid')).toBe('true');
    expect(updateInvoicingSettings).not.toHaveBeenCalled();
  });

  it('is the section the setup notices link to', () => {
    const view = renderForm();
    expect(view.container.querySelector('#points-of-sale')).toBeTruthy();
  });
});

describe('InvoicingSettingsForm gross income', () => {
  it('opens on the number already stored', () => {
    const view = renderForm();
    expect(
      (view.getByLabelText(copy.profile.grossIncomeNumber, { exact: false }) as HTMLInputElement)
        .value,
    ).toBe('901-123456-7');
  });

  it('asks for no number when the corralón is exempt', async () => {
    const view = renderForm();

    fireEvent.click(view.getByRole('radio', { name: copy.profile.grossIncomeKinds.exempt }));
    expect(view.queryByLabelText(copy.profile.grossIncomeNumber, { exact: false })).toBeNull();
    expect(view.getByText(copy.profile.grossIncomeExemptHint)).toBeTruthy();
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith(
        expect.objectContaining({ grossIncomeKind: 'exempt' }),
      ),
    );
  });

  it('opens an exempt registration on the exempt choice', () => {
    const view = renderForm({
      ...SETTINGS,
      profile: { ...SETTINGS.profile!, grossIncomeRegistration: 'EXENTO' },
    });
    const exempt = view.getByRole('radio', { name: copy.profile.grossIncomeKinds.exempt });
    expect(exempt.getAttribute('aria-checked')).toBe('true');
  });

  it('fills the number from the account CUIT on request', () => {
    const view = renderForm({
      ...SETTINGS,
      profile: { ...SETTINGS.profile!, grossIncomeRegistration: '' },
    });

    fireEvent.click(view.getByRole('button', { name: copy.profile.useCuit }));

    expect(
      (view.getByLabelText(copy.profile.grossIncomeNumber, { exact: false }) as HTMLInputElement)
        .value,
    ).toBe('30-71111111-2');
  });

  it('refuses a malformed number on its field', async () => {
    const view = renderForm();
    const number = view.getByLabelText(copy.profile.grossIncomeNumber, { exact: false });

    fireEvent.change(number, { target: { value: '901--123456' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() => expect(number.getAttribute('aria-invalid')).toBe('true'));
    expect(updateInvoicingSettings).not.toHaveBeenCalled();
  });

  // Provinces group the number differently, so it is filtered, never reshaped into one pattern.
  it('keeps only the digits and hyphens of what is typed or pasted', () => {
    const view = renderForm();
    const number = view.getByLabelText(copy.profile.grossIncomeNumber, {
      exact: false,
    }) as HTMLInputElement;

    fireEvent.change(number, { target: { value: ' 901 234.567-8 n' } });

    expect(number.value).toBe('901234567-8');
    expect(number.getAttribute('autocomplete')).toBe('off');
  });
});

describe('InvoicingSettingsForm focus and hints', () => {
  // Focus lands on the first invalid field in reading order, the IVA condition before the table.
  it('focuses the first invalid field after a refused save', async () => {
    const view = renderForm({ ...SETTINGS, ivaCondition: null });

    fireEvent.change(pointOfSale(view, 'Morón'), { target: { value: '3' } });
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    const condition = view.getByRole('combobox', { name: new RegExp(copy.ivaCondition.label) });
    await waitFor(() => expect(document.activeElement).toBe(condition));
  });

  it('explains the price switch with an example that follows its state', () => {
    const view = renderForm();
    const hint = () => view.getByText(/Por ejemplo/).textContent?.replace(/\s/g, ' ') ?? '';

    expect(hint()).toMatch(
      /^Los precios ya incluyen el IVA\. .*\$ 1\.210,00 se factura como \$ 1\.000,00/,
    );
    fireEvent.click(view.getByRole('switch', { name: copy.pricesIncludeVat.label }));
    expect(hint()).toMatch(/^El IVA se suma al facturar\. .*= \$ 1\.210,00\.$/);
  });

  // The default for «Facturar también» is saved with the rest and named in the confirmation.
  it('saves whether marking a sale paid offers its invoice ticked', async () => {
    const view = renderForm();

    fireEvent.click(view.getByRole('switch', { name: copy.invoiceOnPayment.label }));
    fireEvent.click(view.getByRole('button', { name: copy.submit }));

    await waitFor(() =>
      expect(updateInvoicingSettings).toHaveBeenCalledWith(
        expect.objectContaining({ invoiceOnPayment: true }),
      ),
    );
    expect(toast.success).toHaveBeenCalledWith(
      expect.stringContaining(copy.changes.invoiceOnPayment),
    );
  });

  it('offers the account CUIT from the hint under the number, not beside the field', () => {
    const view = renderForm();
    const hint = view.getByText(copy.profile.grossIncomeNumberHint, { exact: false });
    expect(within(hint).getByRole('button', { name: copy.profile.useCuit })).toBeTruthy();
  });
});

describe('InvoicingSettingsForm fiscal identity', () => {
  it('shows the account’s legal name and CUIT read-only, with the way to edit them', () => {
    const view = renderForm();
    const identity = view.getByText(copy.fiscal.legalName).closest('section')!;
    expect(within(identity).getByText('30-71111111-2')).toBeTruthy();
    expect(
      within(identity).getByRole('link', { name: copy.fiscal.editLink }).getAttribute('href'),
    ).toBe('/settings/account');
  });
});
