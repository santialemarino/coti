import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import { ApiError } from '@/lib/api/errors';
import type { Invoice, InvoicePreview } from '@/lib/api/invoicing';
import { getInvoicePreview, issueCreditNote, issueInvoice } from '@/lib/api/invoicing-client';
import messages from '@/translations/es.json';
import { InvoiceCard } from './invoice-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/rfqs/_components/rfq-list-context', () => ({
  useRfqList: () => ({ isAdmin: true }),
}));
vi.mock('@/components/invoice-recovery-button', () => ({
  InvoiceRecoveryButton: vi.fn(() => null),
}));
vi.mock('@/lib/api/invoicing-client', () => ({
  getInvoicePreview: vi.fn(),
  issueInvoice: vi.fn(),
  issueCreditNote: vi.fn(),
}));

const QUOTE_ID = '20000000-0000-4000-8000-000000000031';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const CLIENT_ID = 'c0000000-0000-4000-8000-000000000031';
const card = messages.invoicing.card;

const READY: InvoicePreview = {
  environment: 'PRODUCTION',
  issuerCuit: '30711111112',
  fingerprint: 'a'.repeat(64),
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
  otherEnvironmentInvoice: null,
  creditedInvoice: null,
};

const ISSUED: Invoice = {
  branchId: 'branch-1',
  kind: 'INVOICE',
  environment: 'PRODUCTION',
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
  creditNote: null,
  creditedInvoice: null,
};

const CREDIT_NOTE: Invoice = {
  ...ISSUED,
  id: 'nc-1',
  kind: 'CREDIT_NOTE',
  number: 24,
  issuedOn: '2026-10-07',
  cae: '76543210980024',
  qrUrl: 'https://www.afip.gob.ar/fe/qr/?p=nc',
  creditedInvoice: { id: ISSUED.id, type: 'A', pointOfSale: 3, number: 23, issuedOn: '2026-10-05' },
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
    const fixes = view.getAllByRole('link', {
      name: messages.common.setup.BRANCH_POINT_OF_SALE.fix,
    });
    for (const fix of fixes)
      expect(fix.getAttribute('href')).toBe(ROUTES.invoicingSection('points-of-sale'));
    expect(view.getByText(card.issues.QUOTE_EMPTY.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: /Emitir factura/ })).toBeNull();
  });

  it.each([
    ['ACCOUNT_TAX_ID', ROUTES.accountSettings],
    ['ARCA_CREDENTIALS_CUIT', ROUTES.arcaSettings],
  ] as const)('reports %s as a setup gap with the way to fix it', async (issue, href) => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, issues: [issue] });
    const view = renderCard();

    expect(await view.findByText(messages.common.setup[issue].title)).toBeTruthy();
    expect(view.getByText(messages.common.setup[issue].body, { exact: false })).toBeTruthy();
    expect(
      view.getByRole('link', { name: messages.common.setup[issue].fix }).getAttribute('href'),
    ).toBe(href);
    expect(view.queryByRole('button', { name: /Emitir factura/ })).toBeNull();
  });

  it('links missing fiscal data to the client profile without rendering an order form', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      issues: ['RECEIVER_CUIT_REQUIRED'],
    });
    const view = renderCard();
    const link = await view.findByRole('link', { name: card.editClient });
    expect(link.getAttribute('href')).toBe(ROUTES.clientDetail(CLIENT_ID));
    expect(view.queryByRole('form')).toBeNull();
    expect(view.queryByRole('textbox')).toBeNull();
  });
  it('keeps the ready client profile accessible for correcting invoice details', async () => {
    const view = renderCard();
    const link = await view.findByRole('link', { name: card.editClient });
    expect(link.getAttribute('href')).toBe(ROUTES.clientDetail(CLIENT_ID));
    expect(view.queryByRole('form')).toBeNull();
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
    expect(view.queryByRole('link', { name: card.editClient })).toBeNull();
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

  it('marks an invoice issued in homologation as a test, and nothing in production', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: { ...ISSUED, environment: 'HOMOLOGATION' },
    });
    const test = renderCard();
    expect(await test.findByText(messages.invoicing.testBadge)).toBeTruthy();
    expect(test.getByText(card.testNote)).toBeTruthy();
    test.unmount();

    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, invoice: ISSUED });
    const live = renderCard();
    expect(await live.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(live.queryByText(messages.invoicing.testBadge)).toBeNull();
    expect(live.queryByText(card.testNote)).toBeNull();
  });

  // The invoice keeps the environment it was authorized in, whatever the server says today.
  it('reads the test mark from the issued invoice, not from the server', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      environment: 'HOMOLOGATION',
      invoice: ISSUED,
    });
    const view = renderCard();
    expect(await view.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(view.queryByText(messages.invoicing.testBadge)).toBeNull();
  });

  it('marks a draft as a test while the server invoices in homologation', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, environment: 'HOMOLOGATION' });
    const view = renderCard();
    expect(await view.findByText(messages.invoicing.testBadge)).toBeTruthy();
    expect(view.getByText(card.testNote)).toBeTruthy();
  });

  // The test invoice is history: nothing reads as invoiced, and the real one is offered.
  it('shows a test invoice as history beside the real one to issue', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      otherEnvironmentInvoice: { ...ISSUED, environment: 'HOMOLOGATION' },
    });
    const view = renderCard();

    expect(await view.findByText('Factura A')).toBeTruthy();
    expect(view.getByText('Factura A 0003-00000023')).toBeTruthy();
    expect(view.getByText(messages.invoicing.testBadge)).toBeTruthy();
    expect(view.getByText(/no tiene validez fiscal/)).toBeTruthy();
    expect(view.getByRole('link', { name: card.otherInvoice.download }).getAttribute('href')).toBe(
      ROUTES.invoicePDF(ISSUED.id, BRANCH_ID),
    );
    expect(view.getByRole('button', { name: 'Emitir factura A' })).toBeTruthy();
    expect(view.queryByText(card.testNote)).toBeNull();
  });

  // A pending invoice is badged by its own environment, not the server's.
  it('badges a pending test invoice as a test while the server invoices for real', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: {
        ...ISSUED,
        environment: 'HOMOLOGATION',
        status: 'PENDING',
        number: null,
        cae: null,
      },
    });
    const view = renderCard();
    expect(await view.findByText(card.pending.title)).toBeTruthy();
    expect(view.getByText(messages.invoicing.testBadge)).toBeTruthy();
  });

  it('cancels an issued invoice with a credit note only after the seller confirms', async () => {
    vi.mocked(getInvoicePreview)
      .mockResolvedValueOnce({ ...READY, invoice: ISSUED })
      .mockResolvedValue({ ...READY, creditedInvoice: { ...ISSUED, creditNote: CREDIT_NOTE } });
    vi.mocked(issueCreditNote).mockResolvedValue(CREDIT_NOTE);
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: card.creditNote.issue }));
    expect(issueCreditNote).not.toHaveBeenCalled();
    const dialog = within(await view.findByRole('dialog'));
    expect(
      dialog.getByText(/anula la factura A 0003-00000023 de Constructora Horizonte SA/),
    ).toBeTruthy();
    expect(dialog.getByText(/No se puede deshacer/)).toBeTruthy();

    fireEvent.click(dialog.getByRole('button', { name: card.creditNote.confirm.confirm }));

    await waitFor(() => expect(issueCreditNote).toHaveBeenCalledWith(ISSUED.id, BRANCH_ID));
    expect(
      await view.findByText('Factura A 0003-00000023 anulada por nota de crédito A 0003-00000024'),
    ).toBeTruthy();
    expect(toast.success).toHaveBeenCalledWith(
      'Emitimos la nota de crédito A 0003-00000024: la factura A 0003-00000023 quedó anulada.',
    );
    expect(view.getByRole('button', { name: 'Emitir factura A' })).toBeTruthy();
  });

  // Once credited, the sale shows the note that cancelled the invoice and can be invoiced again.
  it('shows the credited invoice with its credit note above the new draft', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      creditedInvoice: {
        ...ISSUED,
        environment: 'HOMOLOGATION',
        creditNote: { ...CREDIT_NOTE, environment: 'HOMOLOGATION' },
      },
    });
    const view = renderCard();

    expect(
      await view.findByText('Factura A 0003-00000023 anulada por nota de crédito A 0003-00000024'),
    ).toBeTruthy();
    expect(view.getByText(card.credited.body)).toBeTruthy();
    expect(view.getByText(CREDIT_NOTE.cae!)).toBeTruthy();
    expect(view.getByText(messages.invoicing.testBadge)).toBeTruthy();
    expect(view.getByRole('link', { name: card.credited.downloadNote }).getAttribute('href')).toBe(
      ROUTES.invoicePDF(CREDIT_NOTE.id, BRANCH_ID),
    );
    expect(
      view.getByRole('link', { name: card.credited.downloadInvoice }).getAttribute('href'),
    ).toBe(ROUTES.invoicePDF(ISSUED.id, BRANCH_ID));
    expect(view.getByRole('link', { name: card.credited.verify }).getAttribute('href')).toBe(
      CREDIT_NOTE.qrUrl,
    );
    expect(view.getByRole('button', { name: 'Emitir factura A' })).toBeTruthy();
  });

  it('holds the credit note action while ARCA still owes the answer for one', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: { ...ISSUED, creditNote: { ...CREDIT_NOTE, status: 'PENDING', number: null } },
    });
    const view = renderCard();

    expect(await view.findByText(card.creditNote.pending.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: card.creditNote.issue })).toBeNull();
  });

  it('shows ARCA’s reasons for a refused credit note and offers it again', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: {
        ...ISSUED,
        creditNote: { ...CREDIT_NOTE, status: 'REJECTED', number: null, issues: ['10040: Tipo'] },
      },
    });
    const view = renderCard();

    expect(await view.findByText('10040: Tipo')).toBeTruthy();
    expect(view.getByText(card.creditNote.rejected.title)).toBeTruthy();
    expect(view.getByRole('button', { name: card.creditNote.issue })).toBeTruthy();
  });

  it('words a refused credit note as one, not as an invoice', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, invoice: ISSUED });
    vi.mocked(issueCreditNote).mockRejectedValue(new ApiError('INVOICE_ALREADY_CREDITED', 409));
    const view = renderCard();

    fireEvent.click(await view.findByRole('button', { name: card.creditNote.issue }));
    fireEvent.click(
      within(await view.findByRole('dialog')).getByRole('button', {
        name: card.creditNote.confirm.confirm,
      }),
    );

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(messages.errors.INVOICE_ALREADY_CREDITED),
    );
    expect(getInvoicePreview).toHaveBeenCalledTimes(2);
  });

  // A real invoice seen from a test server could only be answered "not ready", so nothing is offered.
  it('offers a credit note only from a server in the invoice’s environment', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      environment: 'HOMOLOGATION',
      invoice: ISSUED,
    });
    const view = renderCard();

    expect(await view.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(view.queryByRole('button', { name: card.creditNote.issue })).toBeNull();
    view.unmount();

    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, invoice: ISSUED });
    const same = renderCard();
    expect(await same.findByRole('button', { name: card.creditNote.issue })).toBeTruthy();
  });
});

describe('InvoiceCard on an archived sale', () => {
  function renderReadOnly() {
    return render(
      <NextIntlClientProvider
        locale="es"
        messages={messages}
        timeZone="America/Argentina/Buenos_Aires"
      >
        <InvoiceCard quoteId={QUOTE_ID} branchId={BRANCH_ID} clientId={CLIENT_ID} readOnly />
      </NextIntlClientProvider>,
    );
  }

  it('still shows the issued invoice, to consult and download', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, invoice: ISSUED });
    const view = renderReadOnly();

    expect(await view.findByText('Factura A 0003-00000023')).toBeTruthy();
    expect(view.getByRole('link', { name: card.verify })).toBeTruthy();
  });

  it('shows what would be issued without offering to issue it', async () => {
    const view = renderReadOnly();

    expect(await view.findByText('Factura A')).toBeTruthy();
    expect(view.queryByRole('button', { name: 'Emitir factura A' })).toBeNull();
  });

  it('reports an invoice in flight without offering to recover it', async () => {
    const { InvoiceRecoveryButton } = await import('@/components/invoice-recovery-button');
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: { ...ISSUED, status: 'PENDING', number: null, cae: null },
    });
    const view = renderReadOnly();

    expect(await view.findByText(card.pending.title)).toBeTruthy();
    expect(InvoiceRecoveryButton).not.toHaveBeenCalled();
  });
});
