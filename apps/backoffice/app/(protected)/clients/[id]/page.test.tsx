import { render, within } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import type { ClientSale } from '@/lib/api/client-profiles';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(async (namespace: string) =>
    createTranslator({ locale: 'es', messages, namespace: namespace as never }),
  ),
}));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: vi.fn(async () => ({ dateNumeric: String, currency: String })),
}));
vi.mock('@/lib/api/account', () => ({ getAccount: vi.fn() }));
vi.mock('@/lib/api/clients', () => ({
  getClient: vi.fn(),
  getClientTags: vi.fn(),
  getClientFiscal: vi.fn(),
}));
vi.mock('@/app/(protected)/_components/page-header', () => ({ PageHeader: () => null }));
vi.mock('@/app/(protected)/clients/[id]/_components/client-registration-link', () => ({
  ClientRegistrationLink: () => null,
}));
vi.mock('@/components/back-link', () => ({ BackLink: () => null }));
vi.mock('@/app/(protected)/clients/[id]/_components/client-tag-editor', () => ({
  ClientTagEditor: () => null,
}));
vi.mock('@/app/(protected)/clients/[id]/_components/client-fiscal-form', () => ({
  ClientFiscalForm: () => <div data-testid="fiscal-form" />,
}));

const { getAccount } = await import('@/lib/api/account');
const { getClient, getClientFiscal, getClientTags } = await import('@/lib/api/clients');
const { default: ClientProfilePage } = await import('./page');

async function renderPage() {
  return render(await ClientProfilePage({ params: Promise.resolve({ id: 'c1' }) }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getClient).mockResolvedValue({
    client: { id: 'c1', name: 'Obra Norte', phone: null, email: null },
    tags: [],
    sales: [],
  } as never);
  vi.mocked(getClientTags).mockResolvedValue([]);
  vi.mocked(getClientFiscal).mockResolvedValue({} as never);
});

describe('ClientProfilePage fiscal data', () => {
  // A client's fiscal data only serves invoicing with Coti, so off it is neither read nor shown.
  it('leaves the fiscal form out while invoicing is off', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: false } as never);

    const view = await renderPage();

    expect(view.queryByTestId('fiscal-form')).toBeNull();
    expect(getClientFiscal).not.toHaveBeenCalled();
  });

  it('shows the fiscal form while invoicing is on', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: true } as never);

    const view = await renderPage();

    expect(view.getByTestId('fiscal-form')).toBeTruthy();
    expect(getClientFiscal).toHaveBeenCalledWith('c1');
  });
});

describe('ClientProfilePage accepted sales', () => {
  const profile = messages.clients.profile;
  const SALES: ClientSale[] = [
    {
      quoteId: 'q1',
      rfqId: 'r1',
      quoteNumber: 42,
      branchId: 'b1',
      branchName: 'Morón',
      total: '1210.00',
      acceptedAt: '2026-10-05T12:00:00Z',
      invoiceStatus: 'PENDING',
    },
    {
      quoteId: 'q2',
      rfqId: 'r2',
      quoteNumber: 43,
      branchId: 'b1',
      branchName: 'Morón',
      total: '800.00',
      acceptedAt: '2026-10-04T12:00:00Z',
      invoiceStatus: 'WAIVED',
    },
  ];

  beforeEach(() => {
    vi.mocked(getClient).mockResolvedValue({
      client: { id: 'c1', name: 'Obra Norte', phone: null, email: null },
      tags: [],
      sales: SALES,
    } as never);
  });

  // The whole row opens the order through one real link, so the keyboard reaches it once per row.
  it('opens each order from a single link stretched over its row', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: false } as never);

    const view = await renderPage();

    const rows = view.getAllByRole('row').slice(1);
    const links = within(rows[0]!).getAllByRole('link');
    expect(links).toHaveLength(1);
    expect(links[0]!.getAttribute('href')).toBe(ROUTES.rfqsDetail('r1'));
    expect(links[0]!.textContent).toBe('#42');
    expect(links[0]!.className).toContain('after:inset-0');
    expect(rows[0]!.className).toContain('relative');
  });

  it('says where each sale stands with its invoice only while invoicing is on', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: true } as never);

    const view = await renderPage();

    expect(view.getByRole('columnheader', { name: profile.table.invoice })).toBeTruthy();
    const rows = view.getAllByRole('row').slice(1);
    expect(within(rows[0]!).getByText(profile.invoiceStatuses.PENDING)).toBeTruthy();
    expect(within(rows[1]!).getByText(profile.invoiceStatuses.WAIVED)).toBeTruthy();
  });

  it('leaves the invoice column out while invoicing is off', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: false } as never);

    const view = await renderPage();

    expect(view.queryByRole('columnheader', { name: profile.table.invoice })).toBeNull();
    expect(view.queryByText(profile.invoiceStatuses.PENDING)).toBeNull();
  });
});
