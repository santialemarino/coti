import { render, screen, within } from '@testing-library/react';
import { createTranslator, NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import type { Invoice } from '@/lib/api/invoicing';
import messages from '@/translations/es.json';

vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/administration',
}));
vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(async (namespace: string) =>
    createTranslator({ locale: 'es', messages, namespace: namespace as never }),
  ),
}));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: vi.fn(async () => ({
    dateNumeric: (iso: string) => `fecha ${iso}`,
    currency: (amount: string) => `$ ${amount}`,
  })),
}));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getEffectiveBranchId: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/api/invoices', () => ({ getInvoices: vi.fn(), getSalesAwaitingInvoice: vi.fn() }));
vi.mock('@/app/(protected)/administration/_components/pending-invoices-table', () => ({
  PendingInvoicesTable: ({
    sales,
    showBranch,
  }: {
    sales: { quoteId: string }[];
    showBranch: boolean;
  }) => (
    <div data-testid="pending-table" data-show-branch={String(showBranch)}>
      {sales.map((sale) => sale.quoteId).join(',')}
    </div>
  ),
}));
vi.mock('@/lib/api/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/account')>()),
  getAccount: vi.fn(),
}));

const { getSession } = await import('@/lib/auth/session');
const { getEffectiveBranchId } = await import('@/lib/auth/branch');
const { getBranches } = await import('@/lib/api/branches');
const { getInvoices, getSalesAwaitingInvoice } = await import('@/lib/api/invoices');
const { getAccount } = await import('@/lib/api/account');
const { default: AdministrationPage } = await import('@/app/(protected)/administration/page');

const copy = messages.invoicing.history;
const pendingCopy = messages.invoicing.pending;
const views = messages.invoicing.views;

function invoice(overrides: Partial<Invoice>): Invoice {
  return {
    id: 'inv-1',
    branchId: 'moron',
    kind: 'INVOICE',
    environment: 'PRODUCTION',
    status: 'ISSUED',
    type: 'B',
    pointOfSale: 3,
    number: 23,
    issuedOn: '2026-10-05',
    cae: '74123456789012',
    caeExpiresOn: '2026-10-15',
    issuerCuit: '30712345679',
    receiver: { name: 'Obra Norte', docType: 'CUIT', docNumber: '1', ivaCondition: 'X' },
    amounts: { net: '1000', exempt: '0', vat: '210', total: '1210.00', byRate: [] },
    currency: 'ARS',
    issues: [],
    qrUrl: '',
    createdAt: '2026-10-05T00:00:00Z',
    creditNote: null,
    creditedInvoice: null,
    ...overrides,
  };
}

async function renderPage(
  branchId: string | undefined,
  items: Invoice[],
  total = items.length,
  searchParams: { page?: string; view?: string } = {},
) {
  vi.mocked(getEffectiveBranchId).mockResolvedValue(branchId);
  vi.mocked(getInvoices).mockResolvedValue({ items, total, page: 1, pageSize: 25 });
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await AdministrationPage({ searchParams: Promise.resolve(searchParams) })}
    </NextIntlClientProvider>,
  );
}

async function renderPending(branchId: string | undefined, total = 2, page?: string) {
  vi.mocked(getSalesAwaitingInvoice).mockResolvedValue({
    items: [{ quoteId: 'q1' }, { quoteId: 'q2' }] as never,
    total,
    page: 1,
    pageSize: 25,
  });
  await renderPage(branchId, [], 0, { view: 'pending', page });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: true, hasInvoices: true } as never);
  vi.mocked(getSession).mockResolvedValue({ role: 'ADMIN' } as never);
  vi.mocked(getBranches).mockResolvedValue([
    { id: 'moron', name: 'Morón' },
    { id: 'bosch', name: 'Villa Bosch' },
  ] as never);
});

describe('AdministrationPage', () => {
  it('is titled apart from the invoicing settings', async () => {
    await renderPage('moron', []);
    expect(screen.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
    expect(copy.title).not.toBe(messages.invoicing.settings.title);
  });

  it('lists every branch’s invoices with a Sucursal column when no branch is selected', async () => {
    await renderPage(undefined, [
      invoice({}),
      invoice({ id: 'inv-2', branchId: 'bosch', status: 'PENDING', number: null }),
    ]);

    expect(screen.getByRole('columnheader', { name: copy.branch })).toBeTruthy();
    const rows = screen.getAllByRole('row').slice(1);
    expect(within(rows[0]!).getByText('Morón')).toBeTruthy();
    expect(within(rows[1]!).getByText('Villa Bosch')).toBeTruthy();
    // Each row acts in its own branch, not in the selection.
    expect(within(rows[0]!).getByRole('link', { name: copy.download }).getAttribute('href')).toBe(
      ROUTES.invoicePDF('inv-1', 'moron'),
    );
    expect(getInvoices).toHaveBeenCalledWith(1);
  });

  it('drops the Sucursal column once one branch is selected', async () => {
    await renderPage('moron', [invoice({})]);
    expect(screen.queryByRole('columnheader', { name: copy.branch })).toBeNull();
    expect(screen.getByText('B 0003-00000023')).toBeTruthy();
    expect(screen.getByText('fecha 2026-10-05')).toBeTruthy();
  });

  it('lists ARCA’s reasons for a refusal one after another', async () => {
    await renderPage('moron', [
      invoice({ status: 'REJECTED', number: null, issues: ['10016: fecha', '10048: importe'] }),
    ]);
    expect(screen.getByText('10016: fecha')).toBeTruthy();
    expect(screen.getByText('10048: importe')).toBeTruthy();
  });

  it('pages with the shared pagination once there is more than one page', async () => {
    await renderPage('moron', [invoice({})], 60);
    expect(screen.getByRole('navigation', { name: messages.common.pagination.label })).toBeTruthy();
  });

  it('says so when no branch has invoices yet', async () => {
    await renderPage(undefined, []);
    expect(screen.getByText(copy.emptyAll)).toBeTruthy();
  });

  // A credit note names the invoice it cancels, and that invoice reads as cancelled.
  it('lists a credit note under its own type, linked to the invoice it cancels', async () => {
    const note = invoice({
      id: 'nc-1',
      kind: 'CREDIT_NOTE',
      number: 4,
      creditedInvoice: {
        id: 'inv-1',
        type: 'B',
        pointOfSale: 3,
        number: 23,
        issuedOn: '2026-10-05',
      },
    });
    await renderPage('moron', [note, invoice({ creditNote: note })]);

    const rows = screen.getAllByRole('row').slice(1);
    expect(within(rows[0]!).getByText('NC B 0003-00000004')).toBeTruthy();
    expect(
      within(rows[0]!).getByRole('link', { name: 'Anula B 0003-00000023' }).getAttribute('href'),
    ).toBe(ROUTES.invoicePDF('inv-1', 'moron'));
    expect(within(rows[0]!).getByText(copy.statuses.ISSUED)).toBeTruthy();
    expect(within(rows[1]!).getByText('B 0003-00000023')).toBeTruthy();
    expect(within(rows[1]!).getByText(copy.statuses.CREDITED)).toBeTruthy();
    expect(within(rows[1]!).queryByText(copy.statuses.ISSUED)).toBeNull();
  });

  it('marks only the invoices issued in homologation as tests', async () => {
    await renderPage('moron', [
      invoice({ environment: 'HOMOLOGATION' }),
      invoice({ id: 'inv-2', number: 24 }),
    ]);
    const rows = screen.getAllByRole('row').slice(1);
    expect(within(rows[0]!).getByText(messages.invoicing.testBadge)).toBeTruthy();
    expect(within(rows[1]!).queryByText(messages.invoicing.testBadge)).toBeNull();
  });
});

describe('AdministrationPage «Por facturar»', () => {
  it('offers both views, with the history chosen by default', async () => {
    await renderPage('moron', []);
    const group = screen.getByRole('radiogroup', { name: views.label });
    expect(
      within(group).getByRole('radio', { name: views.history }).getAttribute('aria-checked'),
    ).toBe('true');
    expect(
      within(group).getByRole('radio', { name: views.pending }).getAttribute('aria-checked'),
    ).toBe('false');
    expect(getSalesAwaitingInvoice).not.toHaveBeenCalled();
  });

  it('lists the sales awaiting an invoice in their own view', async () => {
    await renderPending(undefined);

    expect(screen.getByRole('heading', { level: 1, name: pendingCopy.title })).toBeTruthy();
    expect(screen.getByRole('radio', { name: views.pending }).getAttribute('aria-checked')).toBe(
      'true',
    );
    const table = screen.getByTestId('pending-table');
    expect(table.textContent).toBe('q1,q2');
    expect(table.getAttribute('data-show-branch')).toBe('true');
    expect(getSalesAwaitingInvoice).toHaveBeenCalledWith(1);
    expect(getInvoices).not.toHaveBeenCalled();
  });

  it('pages the view on its own count and drops the Sucursal column inside one branch', async () => {
    await renderPending('moron', 60, '2');
    expect(screen.getByTestId('pending-table').getAttribute('data-show-branch')).toBe('false');
    expect(getSalesAwaitingInvoice).toHaveBeenCalledWith(2);
    expect(screen.getByRole('navigation', { name: messages.common.pagination.label })).toBeTruthy();
  });
});

describe('AdministrationPage with invoicing off', () => {
  // With invoicing off nothing waits for an invoice, so only the history is left.
  it('shows neither the switch nor «Por facturar», even when asked for it', async () => {
    vi.mocked(getAccount).mockResolvedValue({
      invoicingEnabled: false,
      hasInvoices: true,
    } as never);

    await renderPage('moron', [invoice({})], 1, { view: 'pending' });

    expect(screen.queryByRole('radiogroup', { name: views.label })).toBeNull();
    expect(screen.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
    expect(getSalesAwaitingInvoice).not.toHaveBeenCalled();
    expect(screen.getByText('Obra Norte')).toBeTruthy();
  });

  // Nothing was ever issued, so there is no history to show and the section does not exist.
  it('is not found when the account never invoiced', async () => {
    vi.mocked(getAccount).mockResolvedValue({
      invoicingEnabled: false,
      hasInvoices: false,
    } as never);

    await expect(AdministrationPage({ searchParams: Promise.resolve({}) })).rejects.toThrow();
    expect(getInvoices).not.toHaveBeenCalled();
  });

  // Issued invoices are fiscal records: switching invoicing off never hides them.
  it('still lists the invoices already issued', async () => {
    vi.mocked(getAccount).mockResolvedValue({
      invoicingEnabled: false,
      hasInvoices: true,
    } as never);

    await renderPage('moron', [invoice({})]);

    expect(screen.getByText('Obra Norte')).toBeTruthy();
  });
});
