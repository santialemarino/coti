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
vi.mock('@/lib/api/invoices', () => ({ getInvoices: vi.fn() }));

const { getSession } = await import('@/lib/auth/session');
const { getEffectiveBranchId } = await import('@/lib/auth/branch');
const { getBranches } = await import('@/lib/api/branches');
const { getInvoices } = await import('@/lib/api/invoices');
const { default: AdministrationPage } = await import('@/app/(protected)/administration/page');

const copy = messages.invoicing.history;

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

async function renderPage(branchId: string | undefined, items: Invoice[], total = items.length) {
  vi.mocked(getEffectiveBranchId).mockResolvedValue(branchId);
  vi.mocked(getInvoices).mockResolvedValue({ items, total, page: 1, pageSize: 25 });
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await AdministrationPage({ searchParams: Promise.resolve({}) })}
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
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
