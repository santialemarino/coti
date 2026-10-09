import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import { ApiError } from '@/lib/api/errors';
import type { Invoice, InvoicePreview } from '@/lib/api/invoicing';
import {
  getClientFiscal,
  getInvoicePreview,
  issueInvoice,
  updateClientFiscal,
} from '@/lib/api/invoicing-client';
import messages from '@/translations/es.json';
import { InvoiceCard } from './invoice-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/rfqs/_components/rfq-list-context', () => ({
  useRfqList: () => ({ isAdmin: true }),
}));
vi.mock('@/lib/api/invoicing-client', () => ({
  getInvoicePreview: vi.fn(),
  issueInvoice: vi.fn(),
  getClientFiscal: vi.fn(),
  updateClientFiscal: vi.fn(),
}));

const QUOTE_ID = '20000000-0000-4000-8000-000000000031';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const CLIENT_ID = 'c0000000-0000-4000-8000-000000000031';
const card = messages.invoicing.card;

const READY: InvoicePreview = {
  versionId: 'v0000000-0000-4000-8000-000000000031',
  type: 'A',
  pointOfSale: 3,
  receiver: {
    name: 'Constructora Horizonte SA',
    docType: 'CUIT',
    docNumber: '30701234568',
    ivaCondition: 'REGISTERED',
  },
  amounts: {
    net: '1000.00',
    exempt: '0.00',
    vat: '315.00',
    total: '1315.00',
    byRate: [
      { rate: 'VAT_21', base: '500.00', amount: '105.00' },
      { rate: 'VAT_10_5', base: '2000.00', amount: '210.00' },
    ],
  },
  currency: 'ARS',
  issues: [],
  invoice: null,
};

const ISSUED: Invoice = {
  id: 'inv-1',
  status: 'ISSUED',
  type: 'A',
  pointOfSale: 3,
  number: 23,
  issuedOn: '2026-10-05',
  cae: '76543210987654',
  caeExpiresOn: '2026-10-15',
  issuerCuit: '30711111112',
  receiver: READY.receiver,
  amounts: READY.amounts,
  currency: 'ARS',
  issues: [],
  qrUrl: 'https://www.afip.gob.ar/fe/qr/?p=abc',
  createdAt: '2026-10-05T15:00:00Z',
};

function cardWith(clientId: string | null) {
  return (
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <InvoiceCard quoteId={QUOTE_ID} branchId={BRANCH_ID} clientId={clientId} />
    </NextIntlClientProvider>
  );
}

function renderCard(clientId: string | null = CLIENT_ID) {
  return render(cardWith(clientId));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getInvoicePreview).mockResolvedValue(READY);
});

describe('InvoiceCard', () => {
  it('shows an issued invoice with its number, CAE and the way to verify it', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, invoice: ISSUED });
    const view = renderCard();

    expect(await view.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(view.getByText('76543210987654')).toBeTruthy();
    expect(view.getByRole('button', { name: card.caeCopy })).toBeTruthy();
    const verify = view.getByRole('link', { name: card.verify });
    expect(verify.getAttribute('href')).toBe(ISSUED.qrUrl);
    expect(verify.getAttribute('target')).toBe('_blank');
    expect(verify.getAttribute('rel')).toContain('noopener');
    expect(view.queryByRole('button', { name: /Emitir factura/ })).toBeNull();
  });

  it('lays out what would be issued, one row per rate', async () => {
    const view = renderCard();

    expect(await view.findByText('Factura A')).toBeTruthy();
    expect(view.getByText('30-70123456-8', { exact: false })).toBeTruthy();
    const table = within(view.getByRole('table'));
    expect(table.getByText(card.amounts.net)).toBeTruthy();
    expect(table.getByText(messages.invoicing.vatRates.VAT_21)).toBeTruthy();
    expect(table.getByText(messages.invoicing.vatRates.VAT_10_5)).toBeTruthy();
    expect(table.queryByText(card.amounts.exempt)).toBeNull();
    expect(table.getByText(card.amounts.total)).toBeTruthy();
  });

  it('issues only after the seller confirms, then shows the invoice', async () => {
    vi.mocked(issueInvoice).mockResolvedValue(ISSUED);
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: 'Emitir factura A' }));
    expect(issueInvoice).not.toHaveBeenCalled();
    const dialog = within(await view.findByRole('dialog'));
    expect(dialog.getByText(/Constructora Horizonte SA/)).toBeTruthy();
    expect(dialog.getByText(/nota de crédito/)).toBeTruthy();

    fireEvent.click(dialog.getByRole('button', { name: card.confirm.confirm }));

    // The request carries what the seller confirmed, so the API can refuse a sale that moved since.
    await waitFor(() => expect(issueInvoice).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, READY));
    expect(await view.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(toast.success).toHaveBeenCalledWith('Emitimos la factura A 0003-00000023.');
  });

  it('replaces the issue action with what stops it, linking an admin to the fix', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      issues: ['BRANCH_POINT_OF_SALE', 'QUOTE_EMPTY'],
    });
    const view = renderCard();

    expect(await view.findByText(messages.common.setup.BRANCH_POINT_OF_SALE.title)).toBeTruthy();
    const fix = view.getByRole('link', { name: messages.common.setup.BRANCH_POINT_OF_SALE.fix });
    expect(fix.getAttribute('href')).toBe(ROUTES.invoicingSettings);
    expect(view.getByText(card.issues.QUOTE_EMPTY.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: /Emitir factura/ })).toBeNull();
  });

  it('asks for the client’s fiscal data in place and refreshes the preview once saved', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValueOnce({
      ...READY,
      issues: ['RECEIVER_CUIT_REQUIRED'],
    });
    vi.mocked(getClientFiscal).mockResolvedValue({
      id: CLIENT_ID,
      name: 'Horizonte',
      legalName: null,
      taxId: null,
      ivaCondition: 'REGISTERED',
    });
    vi.mocked(updateClientFiscal).mockResolvedValue({
      id: CLIENT_ID,
      name: 'Horizonte',
      legalName: 'Constructora Horizonte SA',
      taxId: '30701234568',
      ivaCondition: 'REGISTERED',
    });
    const view = renderCard();

    const form = within(await view.findByRole('form', { name: card.fiscal.title }));
    fireEvent.change(form.getByLabelText(card.fiscal.legalName.label), {
      target: { value: 'Constructora Horizonte SA' },
    });
    fireEvent.change(form.getByLabelText(card.fiscal.taxId.label), {
      target: { value: '30-70123456-8' },
    });
    fireEvent.click(form.getByRole('button', { name: card.fiscal.submit }));

    await waitFor(() =>
      expect(updateClientFiscal).toHaveBeenCalledWith(CLIENT_ID, {
        legal_name: 'Constructora Horizonte SA',
        tax_id: '30-70123456-8',
        iva_condition: 'REGISTERED',
      }),
    );
    expect(await view.findByRole('button', { name: 'Emitir factura A' })).toBeTruthy();
    expect(getInvoicePreview).toHaveBeenCalledTimes(2);
  });

  it('keeps a ready buyer’s fiscal data one click away, so a registered one can get an A', async () => {
    vi.mocked(getClientFiscal).mockResolvedValue({
      id: CLIENT_ID,
      name: 'Horizonte',
      legalName: null,
      taxId: null,
      ivaCondition: null,
    });
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: card.fiscal.edit }));

    expect(await view.findByRole('form', { name: card.fiscal.title })).toBeTruthy();
    expect(view.queryByRole('button', { name: card.fiscal.edit })).toBeNull();
  });

  it('reloads what would be issued when the sale moved since the seller confirmed', async () => {
    vi.mocked(issueInvoice).mockRejectedValue(new ApiError('INVOICE_STALE', 409));
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: 'Emitir factura A' }));
    const dialog = within(await view.findByRole('dialog'));
    fireEvent.click(dialog.getByRole('button', { name: card.confirm.confirm }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(card.errors.INVOICE_STALE));
    expect(getInvoicePreview).toHaveBeenCalledTimes(2);
  });

  it('keeps the newest preview when an older read answers last', async () => {
    let answerFirst: (preview: InvoicePreview) => void = () => {};
    vi.mocked(getInvoicePreview)
      .mockImplementationOnce(() => new Promise((resolve) => (answerFirst = resolve)))
      .mockResolvedValueOnce({ ...READY, type: 'B' });
    const view = renderCard();
    view.rerender(cardWith(null));

    expect(await view.findByRole('button', { name: 'Emitir factura B' })).toBeTruthy();
    answerFirst(READY);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(view.queryByRole('button', { name: 'Emitir factura A' })).toBeNull();
  });

  it('says a client has to be associated when the sale has none', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, issues: ['RECEIVER_ID_REQUIRED'] });
    const view = renderCard(null);

    expect(await view.findByText(new RegExp(card.noClient))).toBeTruthy();
    expect(view.queryByRole('form')).toBeNull();
    expect(view.queryByRole('button', { name: card.fiscal.edit })).toBeNull();
    expect(getClientFiscal).not.toHaveBeenCalled();
  });

  it('shows ARCA’s reasons verbatim after a refusal and lets the seller retry', async () => {
    const reasons = ['10015: El campo DocNro es inválido.', '10016: El comprobante es duplicado.'];
    vi.mocked(issueInvoice).mockRejectedValue(
      new ApiError('INVOICE_REJECTED', 422, undefined, reasons),
    );
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: 'Emitir factura A' }));
    fireEvent.click(
      within(await view.findByRole('dialog')).getByRole('button', {
        name: card.confirm.confirm,
      }),
    );

    expect(await view.findByText(reasons[0]!)).toBeTruthy();
    expect(view.getByText(reasons[1]!)).toBeTruthy();
    expect(toast.error).toHaveBeenCalledWith(messages.errors.INVOICE_REJECTED);
  });

  it('reads a rejected last attempt off the invoice it already has', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: { ...ISSUED, status: 'REJECTED', number: null, cae: null, issues: ['10013: CUIT'] },
    });
    const view = renderCard();

    expect(await view.findByText('10013: CUIT')).toBeTruthy();
    expect(view.getByText(card.rejected.title)).toBeTruthy();
    expect(view.getByRole('button', { name: 'Emitir factura A' })).toBeTruthy();
  });
});
