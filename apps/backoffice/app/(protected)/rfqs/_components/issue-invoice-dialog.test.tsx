import { fireEvent, render, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { IssueInvoiceDialog } from '@/app/(protected)/rfqs/_components/issue-invoice-dialog';
import type { Invoice, InvoicePreview } from '@/lib/api/invoicing';
import { getInvoicePreview, issueInvoice } from '@/lib/api/invoicing-client';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/invoicing-client', () => ({
  getInvoicePreview: vi.fn(),
  issueInvoice: vi.fn(),
}));

const card = messages.invoicing.card;

const READY: InvoicePreview = {
  environment: 'PRODUCTION',
  issuerCuit: '30711111112',
  fingerprint: 'a'.repeat(64),
  versionId: 'v1',
  type: 'B',
  pointOfSale: 3,
  receiver: { name: 'Obra Norte', docType: 'NONE', docNumber: '', ivaCondition: 'FINAL_CONSUMER' },
  amounts: { net: '1000.00', exempt: '0.00', vat: '210.00', total: '1210.00', byRate: [] },
  currency: 'ARS',
  issues: [],
  invoice: null,
  otherEnvironmentInvoice: null,
  creditedInvoice: null,
};

function renderDialog(preview?: InvoicePreview) {
  const onIssued = vi.fn();
  const onFailed = vi.fn();
  const onOpenChange = vi.fn();
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <IssueInvoiceDialog
        open
        onOpenChange={onOpenChange}
        quoteId="q1"
        branchId="b1"
        preview={preview}
        onIssued={onIssued}
        onFailed={onFailed}
      />
    </NextIntlClientProvider>,
  );
  return { dialog: within(view.getByRole('dialog')), onIssued, onFailed, onOpenChange };
}

beforeEach(() => vi.clearAllMocks());

describe('IssueInvoiceDialog', () => {
  // From the queue there is no draft on screen, so the dialog reads it before offering to issue.
  it('reads the draft when it has none, then issues what it showed', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue(READY);
    vi.mocked(issueInvoice).mockResolvedValue({ type: 'B', pointOfSale: 3, number: 7 } as Invoice);
    const { dialog, onIssued, onOpenChange } = renderDialog();

    expect(dialog.getByText(card.loading)).toBeTruthy();
    await vi.waitFor(() => expect(dialog.getByText(/Obra Norte/)).toBeTruthy());
    fireEvent.click(dialog.getByRole('button', { name: card.confirm.confirm }));

    await vi.waitFor(() => expect(onIssued).toHaveBeenCalled());
    expect(getInvoicePreview).toHaveBeenCalledWith('q1', 'b1');
    expect(issueInvoice).toHaveBeenCalledWith('q1', 'b1', READY);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(toast.success).toHaveBeenCalled();
  });

  it('uses the draft it is handed without reading it again', () => {
    const { dialog } = renderDialog(READY);

    expect(getInvoicePreview).not.toHaveBeenCalled();
    expect(dialog.getByRole('button', { name: card.confirm.confirm })).toBeTruthy();
  });

  // A gap the draft reports is fixed on the order, so the queue offers no way to issue past it.
  it('names what blocks the invoice and offers no confirmation', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({ ...READY, issues: ['QUOTE_EMPTY'] });
    const { dialog } = renderDialog();

    await vi.waitFor(() =>
      expect(dialog.getByText(new RegExp(card.issues.QUOTE_EMPTY.title))).toBeTruthy(),
    );
    expect(dialog.queryByRole('button', { name: card.confirm.confirm })).toBeNull();
  });

  it('offers no second invoice beside one already issued or in flight', async () => {
    vi.mocked(getInvoicePreview).mockResolvedValue({
      ...READY,
      invoice: { status: 'PENDING' } as Invoice,
    });
    const { dialog } = renderDialog();

    await vi.waitFor(() => expect(dialog.getByText(card.confirm.alreadyIssued)).toBeTruthy());
    expect(dialog.queryByRole('button', { name: card.confirm.confirm })).toBeNull();
  });

  it('says the draft could not be read', async () => {
    vi.mocked(getInvoicePreview).mockRejectedValue(new Error('down'));
    const { dialog } = renderDialog();

    await vi.waitFor(() => expect(dialog.getByText(card.confirm.unavailable)).toBeTruthy());
  });

  it('reports a refused issue and hands the failure back', async () => {
    const refusal = new Error('refused');
    vi.mocked(issueInvoice).mockRejectedValue(refusal);
    const { dialog, onFailed, onIssued } = renderDialog(READY);

    fireEvent.click(dialog.getByRole('button', { name: card.confirm.confirm }));

    await vi.waitFor(() => expect(onFailed).toHaveBeenCalledWith(refusal));
    expect(onIssued).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalled();
  });
});
