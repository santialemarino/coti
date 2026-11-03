import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import type { Invoice, InvoicePreview } from '@/lib/api/invoicing';
import { getInvoicePreview, issueInvoice } from '@/lib/api/invoicing-client';
import { setQuotePaid } from '@/lib/api/rfqs-client';
import messages from '@/translations/es.json';
import { SaleClosureCard } from './sale-closure-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/invoicing-client', () => ({
  getInvoicePreview: vi.fn(),
  issueInvoice: vi.fn(),
}));
vi.mock('@/lib/api/rfqs-client', () => ({ setQuotePaid: vi.fn() }));

const QUOTE_ID = '20000000-0000-4000-8000-000000000041';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const copy = messages.rfqs.detail.closure;

const PREVIEW: InvoicePreview = {
  environment: 'PRODUCTION',
  issuerCuit: '30711111112',
  fingerprint: 'b'.repeat(64),
  versionId: 'v0000000-0000-4000-8000-000000000041',
  type: 'B',
  pointOfSale: 3,
  receiver: {
    name: 'Obra Norte',
    docType: 'DNI',
    docNumber: '30123456',
    ivaCondition: 'FINAL_CONSUMER',
  },
  amounts: { net: '1000.00', exempt: '0.00', vat: '210.00', total: '1210.00', byRate: [] },
  currency: 'ARS',
  issues: [],
  invoice: null,
  otherEnvironmentInvoice: null,
  creditedInvoice: null,
  customerSignupUrl: null,
};
const ISSUED = { id: 'inv-1', status: 'ISSUED', type: 'B', pointOfSale: 3, number: 7 } as Invoice;

const onChanged = vi.fn(async () => undefined);

function renderCard(
  props: Partial<React.ComponentProps<typeof SaleClosureCard>> = {},
): ReturnType<typeof render> {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <SaleClosureCard
        quoteId={QUOTE_ID}
        branchId={BRANCH_ID}
        reference="#41"
        paidAt={null}
        paidBy={null}
        saleComplete={false}
        invoicingEnabled
        invoiceOnPayment={false}
        invoiceWaived={false}
        onChanged={onChanged}
        {...props}
      />
    </NextIntlClientProvider>,
  );
}

async function openDialog(view: ReturnType<typeof render>) {
  fireEvent.click(view.getByRole('button', { name: copy.markPaid }));
  return within(await view.findByRole('dialog'));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(setQuotePaid).mockResolvedValue({} as never);
});

describe('SaleClosureCard', () => {
  it('offers «Marcar cobrado» on an unpaid sale', () => {
    const view = renderCard();

    expect(view.getByText(copy.unpaid)).toBeTruthy();
    expect(view.getByRole('button', { name: copy.markPaid })).toBeTruthy();
    expect(view.queryByText(copy.complete)).toBeNull();
  });

  it('shows who marked it paid and takes the mark back', async () => {
    const view = renderCard({
      paidAt: '2026-10-20T15:00:00.000Z',
      paidBy: 'Ana Pérez',
      saleComplete: true,
    });

    expect(view.getByText(/Cobrada por Ana Pérez/)).toBeTruthy();
    expect(view.getByText(copy.complete)).toBeTruthy();
    fireEvent.click(view.getByRole('button', { name: copy.unmarkPaid }));

    await waitFor(() => expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, false, BRANCH_ID));
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
  });

  it('offers no change on an archived sale', () => {
    const view = renderCard({ readOnly: true });

    expect(view.queryByRole('button', { name: copy.markPaid })).toBeNull();
  });
});

describe('MarkPaidDialog', () => {
  it('never offers the invoice while the account does not invoice', async () => {
    const view = renderCard({ invoicingEnabled: false, invoiceOnPayment: true });
    const dialog = await openDialog(view);

    expect(dialog.queryByRole('checkbox')).toBeNull();
    expect(getInvoicePreview).not.toHaveBeenCalled();
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirm }));

    await waitFor(() => expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, true, BRANCH_ID));
    expect(issueInvoice).not.toHaveBeenCalled();
  });

  // The seller declined the invoice; the default must not tick it back on behind their back.
  it('offers no invoice on a sale marked «No requiere factura», whatever the default', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    const view = renderCard({ invoiceOnPayment: true, invoiceWaived: true });
    const dialog = await openDialog(view);

    expect(dialog.queryByRole('checkbox')).toBeNull();
    expect(dialog.getByText(copy.dialog.invoiceWaived)).toBeTruthy();
    expect(dialog.queryByRole('button', { name: copy.dialog.confirmAndInvoice })).toBeNull();
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirm }));

    await waitFor(() => expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, true, BRANCH_ID));
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(issueInvoice).not.toHaveBeenCalled();
    expect(getInvoicePreview).not.toHaveBeenCalled();
  });

  it('starts ticked from the account default and issues after the paid mark', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    vi.mocked(issueInvoice).mockResolvedValue(ISSUED);
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    const checkbox = await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    expect(checkbox.getAttribute('aria-checked')).toBe('true');
    expect(dialog.getByText(/factura B a Obra Norte/)).toBeTruthy();
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirmAndInvoice }));

    await waitFor(() => expect(issueInvoice).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, PREVIEW));
    expect(vi.mocked(setQuotePaid).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(issueInvoice).mock.invocationCallOrder[0]!,
    );
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('0003-00000007'));
  });

  it('starts unticked without the default and then marks paid alone', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    const view = renderCard();
    const dialog = await openDialog(view);

    const checkbox = await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    expect(checkbox.getAttribute('aria-checked')).toBe('false');
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirm }));

    await waitFor(() => expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, true, BRANCH_ID));
    expect(issueInvoice).not.toHaveBeenCalled();
  });

  // A gap the preview reports stops the invoice, and the dialog says which.
  it('disables the invoice and says why when the preview is blocked', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...PREVIEW,
      issues: ['RECEIVER_CUIT_REQUIRED'],
    });
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    const checkbox = await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    expect(checkbox).toHaveProperty('disabled', true);
    expect(checkbox.getAttribute('aria-checked')).toBe('false');
    expect(
      dialog.getByText(new RegExp(messages.invoicing.card.issues.RECEIVER_CUIT_REQUIRED.title)),
    ).toBeTruthy();
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirm }));

    await waitFor(() => expect(setQuotePaid).toHaveBeenCalledOnce());
    expect(issueInvoice).not.toHaveBeenCalled();
  });

  it('hides the invoice when the sale already has one', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...PREVIEW, invoice: ISSUED });
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    await waitFor(() => expect(dialog.queryByText(copy.dialog.invoiceLoading)).toBeNull());
    expect(dialog.queryByRole('checkbox')).toBeNull();
  });

  // The paid mark is committed first, so an invoice ARCA refuses leaves it in place.
  it('keeps the paid mark when the invoice is refused', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    vi.mocked(issueInvoice).mockRejectedValue(new ApiError('INVOICE_REJECTED', 422));
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirmAndInvoice }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.invoiceRejected));
    expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, true, BRANCH_ID);
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
  });

  it('says the invoice did not go out when ARCA cannot be reached', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    vi.mocked(issueInvoice).mockRejectedValue(new ApiError('INVOICING_UNAVAILABLE', 503));
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirmAndInvoice }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.invoiceFailed));
    expect(setQuotePaid).toHaveBeenCalledWith(QUOTE_ID, true, BRANCH_ID);
  });

  it('marks nothing and keeps the dialog when the paid mark is refused', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(PREVIEW);
    vi.mocked(setQuotePaid).mockRejectedValue(new ApiError('QUOTE_ARCHIVED', 409));
    const view = renderCard({ invoiceOnPayment: true });
    const dialog = await openDialog(view);

    await dialog.findByRole('checkbox', { name: copy.dialog.invoice });
    fireEvent.click(dialog.getByRole('button', { name: copy.dialog.confirmAndInvoice }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.QUOTE_ARCHIVED));
    expect(issueInvoice).not.toHaveBeenCalled();
    expect(onChanged).not.toHaveBeenCalled();
  });
});
