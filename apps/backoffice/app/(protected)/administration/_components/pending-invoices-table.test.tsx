import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { PendingInvoicesTable } from '@/app/(protected)/administration/_components/pending-invoices-table';
import { ROUTES } from '@/config/routes';
import { ApiError } from '@/lib/api/errors';
import type { SaleAwaitingInvoice } from '@/lib/api/invoices';
import { setInvoiceWaived } from '@/lib/api/rfqs-client';
import messages from '@/translations/es.json';

const refresh = vi.fn();

vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ push: vi.fn(), refresh }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ setInvoiceWaived: vi.fn() }));
vi.mock('@/app/(protected)/rfqs/_components/issue-invoice-dialog', () => ({
  IssueInvoiceDialog: (props: { open: boolean; quoteId: string; branchId: string }) =>
    props.open ? (
      <div role="dialog" data-quote={props.quoteId} data-branch={props.branchId} />
    ) : null,
}));

const copy = messages.invoicing.pending;
const closing = messages.rfqs.list.closing;

function sale(overrides: Partial<SaleAwaitingInvoice>): SaleAwaitingInvoice {
  return {
    quoteId: 'q1',
    rfqId: 'r1',
    quoteNumber: 42,
    branchId: 'moron',
    branchName: 'Morón',
    clientId: 'c1',
    client: 'Obra Norte',
    total: '1210.00',
    acceptedAt: '2026-10-05T12:00:00Z',
    paidAt: '2026-10-06T12:00:00Z',
    ...overrides,
  };
}

const SALES = [
  sale({}),
  sale({
    quoteId: 'q2',
    rfqId: 'r2',
    quoteNumber: 43,
    branchId: 'bosch',
    branchName: 'Villa Bosch',
    client: '',
    paidAt: null,
  }),
];

function renderTable(sales: SaleAwaitingInvoice[], showBranch = false) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <PendingInvoicesTable sales={sales} showBranch={showBranch} />
    </NextIntlClientProvider>,
  );
}

function rows() {
  return screen.getAllByRole('row').slice(1);
}

beforeEach(() => vi.clearAllMocks());

describe('PendingInvoicesTable', () => {
  it('lists each sale with a link to its order, its client and whether it was paid', () => {
    renderTable(SALES);

    const [paid, unpaid] = rows();
    expect(within(paid!).getByRole('link', { name: '#42' }).getAttribute('href')).toBe(
      ROUTES.rfqsDetail('r1'),
    );
    expect(within(paid!).getByText('Obra Norte')).toBeTruthy();
    expect(within(paid!).getByText(copy.paid)).toBeTruthy();
    expect(within(unpaid!).getByText(copy.noClient)).toBeTruthy();
    expect(within(unpaid!).getByText(copy.unpaid)).toBeTruthy();
    expect(screen.queryByRole('columnheader', { name: copy.branch })).toBeNull();
  });

  it('names each row’s branch while every branch is in view', () => {
    renderTable(SALES, true);
    expect(screen.getByRole('columnheader', { name: copy.branch })).toBeTruthy();
    expect(within(rows()[1]!).getByText('Villa Bosch')).toBeTruthy();
  });

  // Issuing goes through the shared confirmation, in the sale's own branch.
  it('opens the invoice confirmation for the row’s sale', () => {
    renderTable(SALES);
    fireEvent.click(
      within(rows()[1]!).getByRole('button', {
        name: closing.labelled.replace('{action}', closing.INVOICE).replace('{id}', '#43'),
      }),
    );
    const dialog = screen.getByRole('dialog');
    expect(dialog.getAttribute('data-quote')).toBe('q2');
    expect(dialog.getAttribute('data-branch')).toBe('bosch');
  });

  it('marks a sale as needing no invoice, takes it off the list and reads the page again', async () => {
    vi.mocked(setInvoiceWaived).mockResolvedValue({} as never);
    renderTable(SALES);

    fireEvent.click(
      within(rows()[0]!).getByRole('button', {
        name: closing.labelled.replace('{action}', closing.WAIVE).replace('{id}', '#42'),
      }),
    );

    await waitFor(() => expect(rows()).toHaveLength(1));
    expect(setInvoiceWaived).toHaveBeenCalledWith('q1', true, 'moron');
    expect(within(rows()[0]!).getByRole('link', { name: '#43' })).toBeTruthy();
    expect(toast.success).toHaveBeenCalledWith(
      messages.invoicing.card.waiver.waived.replace('{reference}', '#42'),
    );
    expect(refresh).toHaveBeenCalled();
  });

  // A waiver undone elsewhere brings the sale back: the server's list wins over the hidden row.
  it('shows again whatever the next refresh still lists', async () => {
    vi.mocked(setInvoiceWaived).mockResolvedValue({} as never);
    const { rerender } = renderTable(SALES);

    fireEvent.click(
      within(rows()[0]!).getByRole('button', {
        name: closing.labelled.replace('{action}', closing.WAIVE).replace('{id}', '#42'),
      }),
    );
    await waitFor(() => expect(rows()).toHaveLength(1));

    rerender(
      <NextIntlClientProvider locale="es" messages={messages}>
        <PendingInvoicesTable sales={[...SALES]} showBranch={false} />
      </NextIntlClientProvider>,
    );

    expect(rows()).toHaveLength(2);
    expect(within(rows()[0]!).getByRole('link', { name: '#42' })).toBeTruthy();
  });

  it('keeps the sale and says why when the waiver is refused', async () => {
    vi.mocked(setInvoiceWaived).mockRejectedValue(new ApiError('QUOTE_ALREADY_INVOICED', 409));
    renderTable(SALES);

    fireEvent.click(
      within(rows()[0]!).getByRole('button', {
        name: closing.labelled.replace('{action}', closing.WAIVE).replace('{id}', '#42'),
      }),
    );

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(messages.errors.QUOTE_ALREADY_INVOICED),
    );
    expect(rows()).toHaveLength(2);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('says so when nothing waits for an invoice', () => {
    renderTable([], true);
    expect(screen.getByText(copy.emptyAll)).toBeTruthy();
    expect(screen.queryByRole('table')).toBeNull();
  });
});
