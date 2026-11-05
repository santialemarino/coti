import type { ComponentProps } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { createTranslator, NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import type { SellerReport } from '@/lib/api/reports';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(async (namespace: string) =>
    createTranslator({ locale: 'es', messages, namespace: namespace as never }),
  ),
}));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: vi.fn(async () => ({
    value: (value: number) => String(value),
    currency: (amount: string) => `$ ${amount}`,
    ratePct: (ratio: number) => `${Math.round(ratio * 100)}%`,
    date: (iso: string) => `fecha ${iso}`,
    dateNumeric: (iso: string) => `num ${iso}`,
    month: (iso: string) => `mes ${iso}`,
  })),
}));
vi.mock('@/lib/api/reports', () => ({ getSellerReport: vi.fn() }));
const replace = vi.fn();
vi.mock('next/navigation', () => ({
  usePathname: () => '/reports',
  useRouter: () => ({ replace }),
  useSearchParams: () => new URLSearchParams('from=2026-09-26&to=2026-10-25'),
}));
vi.mock('next/form', () => ({
  default: (props: ComponentProps<'form'>) => <form {...props} />,
}));
vi.mock('@/lib/api/clients', () => ({ getClients: vi.fn(), getClientTags: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getActiveBranchId: vi.fn() }));
vi.mock('@/lib/api/users', () => ({ getSellers: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { getSellerReport } = await import('@/lib/api/reports');
const { getClients, getClientTags } = await import('@/lib/api/clients');
const { getBranches } = await import('@/lib/api/branches');
const { getActiveBranchId } = await import('@/lib/auth/branch');
const { getSellers } = await import('@/lib/api/users');
const { getSession } = await import('@/lib/auth/session');
const { default: ReportsPage } = await import('@/app/(protected)/reports/page');

const copy = messages.reports;

function report(overrides: Partial<SellerReport> = {}): SellerReport {
  return {
    range: { from: '2026-09-26', to: '2026-10-25' },
    granularity: 'week',
    scope: { multiBranch: false, branchId: 'branch-1' },
    filters: {
      statuses: [],
      channel: null,
      sellerId: null,
      sellerName: '',
      clientId: null,
      clientName: '',
      branches: [],
      tags: [],
    },
    kpis: {
      rfqsReceived: 12,
      quotesSent: 4,
      quotesAccepted: 3,
      sentAccepted: 2,
      conversionRate: 0.5,
      acceptedAmount: '1250.50',
      responseTime: { averageSeconds: 900, medianSeconds: 600, sample: 3 },
    },
    branches: [],
    quotesByStatus: [
      { status: 'SENT', count: 2 },
      { status: 'ACCEPTED', count: 2 },
    ],
    timeline: [
      { date: '2026-09-26', rfqs: 3, sent: 1, accepted: 0, sentAccepted: 0, conversionRate: 0 },
      { date: '2026-09-28', rfqs: 5, sent: 2, accepted: 1, sentAccepted: 1, conversionRate: 0.5 },
      { date: '2026-10-05', rfqs: 0, sent: 0, accepted: 0, sentAccepted: 0, conversionRate: null },
    ],
    topProducts: [{ productId: 'product-1', name: 'Cemento', requests: 7 }],
    topRevenueProducts: [{ productId: 'product-1', name: 'Cemento', revenue: '1000.00' }],
    topClients: [{ clientId: 'client-1', name: 'Obra Norte', acceptedAmount: '1250.50' }],
    aiMatch: {
      matched: 8,
      total: 10,
      rate: 0.8,
      confidence: { low: 1, medium: 1, high: 8 },
    },
    ...overrides,
  };
}

async function renderReport(
  data: SellerReport,
  searchParams: { from?: string; to?: string; [key: string]: string | string[] | undefined } = {},
) {
  vi.mocked(getSellerReport).mockResolvedValue(data);
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await ReportsPage({ searchParams: Promise.resolve(searchParams) })}
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getSession).mockResolvedValue({
    userId: 'user-1',
    accountId: 'account-1',
    name: 'Ana',
    email: 'ana@example.com',
    emailVerified: true,
    role: 'ADMIN',
    emailVerificationRequired: false,
    mailDelivery: true,
  });
  vi.mocked(getClients).mockResolvedValue([]);
  vi.mocked(getSellers).mockResolvedValue([]);
  vi.mocked(getClientTags).mockResolvedValue([]);
  vi.mocked(getBranches).mockResolvedValue([]);
  vi.mocked(getActiveBranchId).mockResolvedValue('branch-1');
});

describe('ReportsPage', () => {
  it('shows the period figures and limits the report to the selected branch', async () => {
    await renderReport(report());

    expect(screen.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
    expect(screen.getByRole('button', { name: copy.download.label })).toBeTruthy();
    expect(screen.getByText(copy.kpis.rfqs)).toBeTruthy();
    expect(screen.getByText('12')).toBeTruthy();
    expect(screen.getAllByText('$ 1250.50')).toHaveLength(2);
    expect(screen.getAllByText('50%').length).toBeGreaterThan(0);
    expect(screen.getByText(/10 minutos/)).toBeTruthy();
    expect(screen.queryByText(copy.branches.title)).toBeNull();
  });

  it('shows a status pie chart and ranks accepted product revenue', async () => {
    await renderReport(report());

    expect(screen.getByRole('img', { name: copy.status.aria })).toBeTruthy();
    expect(screen.getByText(copy.revenueProducts.title)).toBeTruthy();
    expect(screen.getByText(copy.revenueProducts.description)).toBeTruthy();
    expect(screen.getByText('$ 1000.00')).toBeTruthy();
    expect(screen.getByText('7 líneas')).toBeTruthy();
  });

  it('passes the requested dates and seeds the filters from the returned range', async () => {
    await renderReport(report(), { from: '2026-10-01', to: '2026-10-02' });

    expect(getSellerReport).toHaveBeenCalledWith({
      from: '2026-10-01',
      to: '2026-10-02',
      statuses: [],
      channel: undefined,
      sellerId: undefined,
      clientId: undefined,
      branchIds: [],
      tagIds: [],
    });
    expect((screen.getByLabelText(copy.filters.from) as HTMLInputElement).value).toBe('2026-09-26');
    expect((screen.getByLabelText(copy.filters.to) as HTMLInputElement).value).toBe('2026-10-25');
  });

  it('forwards every report filter and seeds the controls from the URL', async () => {
    vi.mocked(getClients).mockResolvedValue([{ id: 'client-1', name: 'Obra Norte' }] as never);
    vi.mocked(getSellers).mockResolvedValue([{ id: 'seller-1', name: 'Ana' }]);
    const { container } = await renderReport(
      report({
        filters: {
          statuses: ['SENT', 'ACCEPTED'],
          channel: 'EMAIL',
          sellerId: 'seller-1',
          sellerName: 'Ana',
          clientId: 'client-1',
          clientName: 'Obra Norte',
          branches: [],
          tags: [],
        },
      }),
      {
        status: 'SENT,ACCEPTED',
        channel: 'EMAIL',
        seller_id: 'seller-1',
        client_id: 'client-1',
      },
    );

    expect(getSellerReport).toHaveBeenCalledWith({
      from: undefined,
      to: undefined,
      statuses: ['SENT', 'ACCEPTED'],
      channel: 'EMAIL',
      sellerId: 'seller-1',
      clientId: 'client-1',
      branchIds: [],
      tagIds: [],
    });
    expect(
      [...container.querySelectorAll<HTMLInputElement>('input[name="status"]')].map(
        (input) => input.value,
      ),
    ).toEqual(['SENT', 'ACCEPTED']);
    expect(container.querySelector<HTMLInputElement>('input[name="channel"]')?.value).toBe('EMAIL');
    expect(container.querySelector<HTMLInputElement>('input[name="seller_id"]')?.value).toBe(
      'seller-1',
    );
    expect(container.querySelector<HTMLInputElement>('input[name="client_id"]')?.value).toBe(
      'client-1',
    );
  });

  it('narrows by tag and, when no branch is selected, by branch', async () => {
    vi.mocked(getActiveBranchId).mockResolvedValue(undefined);
    vi.mocked(getClientTags).mockResolvedValue([
      { id: 'tag-1', name: 'Mayorista', createdAt: '2026-10-01T00:00:00Z' },
    ]);
    vi.mocked(getBranches).mockResolvedValue([
      {
        id: 'moron',
        name: 'Morón',
        address: null,
        email: null,
        defaultExpiryDays: 7,
        isActive: true,
      },
      {
        id: 'bosch',
        name: 'Villa Bosch',
        address: null,
        email: null,
        defaultExpiryDays: 7,
        isActive: true,
      },
    ]);
    const { container } = await renderReport(
      report({
        scope: { multiBranch: true, branchId: null },
        filters: {
          ...report().filters,
          branches: [{ id: 'moron', name: 'Morón' }],
          tags: [{ id: 'tag-1', name: 'Mayorista' }],
        },
      }),
      { branch_id: 'moron', tag_id: 'tag-1' },
    );

    expect(getSellerReport).toHaveBeenCalledWith(
      expect.objectContaining({ branchIds: ['moron'], tagIds: ['tag-1'] }),
    );
    expect(screen.getByRole('combobox', { name: copy.filters.branch })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: copy.filters.tag })).toBeTruthy();
    expect(container.querySelector<HTMLInputElement>('input[name="branch_id"]')?.value).toBe(
      'moron',
    );
    expect(container.querySelector<HTMLInputElement>('input[name="tag_id"]')?.value).toBe('tag-1');
  });

  it('offers no branch filter once a branch is selected', async () => {
    vi.mocked(getBranches).mockResolvedValue([
      {
        id: 'moron',
        name: 'Morón',
        address: null,
        email: null,
        defaultExpiryDays: 7,
        isActive: true,
      },
      {
        id: 'bosch',
        name: 'Villa Bosch',
        address: null,
        email: null,
        defaultExpiryDays: 7,
        isActive: true,
      },
    ]);
    await renderReport(report());

    expect(screen.queryByRole('combobox', { name: copy.filters.branch })).toBeNull();
  });

  it('hides the seller filter from a non-administrator', async () => {
    vi.mocked(getSession).mockResolvedValue({
      userId: 'user-1',
      accountId: 'account-1',
      name: 'Ana',
      email: 'ana@example.com',
      emailVerified: true,
      role: 'SELLER',
      emailVerificationRequired: false,
      mailDelivery: true,
    });
    await renderReport(report());

    expect(screen.queryByRole('button', { name: copy.filters.seller })).toBeNull();
    expect(getSellers).not.toHaveBeenCalled();
  });

  it('says when the period has no activity instead of showing zeroes as data', async () => {
    await renderReport(
      report({
        kpis: {
          rfqsReceived: 0,
          quotesSent: 0,
          quotesAccepted: 0,
          sentAccepted: 0,
          conversionRate: null,
          acceptedAmount: '0',
          responseTime: { averageSeconds: null, medianSeconds: null, sample: 0 },
        },
        quotesByStatus: [],
        topProducts: [],
        topRevenueProducts: [],
        topClients: [],
        aiMatch: { matched: 0, total: 0, rate: null, confidence: { low: 0, medium: 0, high: 0 } },
      }),
    );

    expect(screen.getByText(copy.empty.title)).toBeTruthy();
    expect(screen.queryByText(copy.kpis.rfqs)).toBeNull();
  });

  it('breaks order volume down by branch only when every branch is in view', async () => {
    await renderReport(
      report({
        scope: { multiBranch: true, branchId: null },
        branches: [
          { branchId: 'moron', branchName: 'Morón', rfqs: 8 },
          { branchId: 'bosch', branchName: 'Villa Bosch', rfqs: 4 },
        ],
      }),
    );

    expect(screen.getByText(copy.branches.title)).toBeTruthy();
    expect(screen.getByText('Morón')).toBeTruthy();
    expect(screen.getByText('Villa Bosch')).toBeTruthy();
  });

  it('lists the accepted commercial amount without pretending it is revenue', async () => {
    await renderReport(report());

    expect(screen.getByText(copy.kpis.acceptedAmountDetail)).toBeTruthy();
    expect(screen.getByText('Obra Norte')).toBeTruthy();
    expect(screen.getAllByText('Cemento').length).toBeGreaterThan(0);
    expect(screen.getByText('80%')).toBeTruthy();
  });

  it("reads conversion as the share of the period's sends accepted so far", async () => {
    await renderReport(report());

    expect(screen.getByText('2 aceptadas de 4 enviadas en el período')).toBeTruthy();
    expect(screen.getByText(copy.kpis.acceptedDetail)).toBeTruthy();
  });

  it('shows no conversion rather than dividing by zero', async () => {
    const base = report();
    await renderReport(
      report({
        kpis: {
          ...base.kpis,
          quotesSent: 0,
          sentAccepted: 0,
          quotesAccepted: 2,
          conversionRate: null,
        },
      }),
    );

    expect(screen.getByText('0 aceptadas de 0 enviadas en el período')).toBeTruthy();
  });

  it('labels trend buckets by the granularity the server applied and switches it in the URL', async () => {
    await renderReport(report());

    expect(screen.getAllByText('Semana del num 2026-09-28').length).toBeGreaterThan(0);
    expect(screen.getByText(copy.timeline.by.week)).toBeTruthy();
    const month = screen.getByRole('radio', { name: copy.timeline.granularity.month });
    fireEvent.click(month);
    expect(replace).toHaveBeenCalledWith(
      '/reports?from=2026-09-26&to=2026-10-25&granularity=month',
      { scroll: false },
    );
  });

  it('labels a monthly trend by month', async () => {
    await renderReport(report({ granularity: 'month' }));

    expect(screen.getAllByText('mes 2026-09-26').length).toBeGreaterThan(0);
  });

  it('names quote statuses as quotes and dates the timeline for a reader', async () => {
    await renderReport(report());

    expect(screen.getAllByText(copy.statusLabels.SENT).length).toBeGreaterThan(0);
    expect(screen.queryByText(messages.rfqs.status.SENT)).toBeNull();
    expect(
      screen.getByRole('img', {
        name: copy.timeline.aria
          .replace('{from}', 'fecha 2026-09-26')
          .replace('{to}', 'fecha 2026-10-25'),
      }),
    ).toBeTruthy();
  });

  it('shows the status section when quotes were created but no order arrived in the period', async () => {
    await renderReport(
      report({
        kpis: {
          rfqsReceived: 0,
          quotesSent: 0,
          quotesAccepted: 0,
          sentAccepted: 0,
          conversionRate: null,
          acceptedAmount: '0',
          responseTime: { averageSeconds: null, medianSeconds: null, sample: 0 },
        },
        quotesByStatus: [{ status: 'DRAFT', count: 1 }],
        topProducts: [],
        topRevenueProducts: [],
        topClients: [],
        aiMatch: { matched: 0, total: 0, rate: null, confidence: { low: 0, medium: 0, high: 0 } },
      }),
    );

    expect(screen.getByText(copy.status.title)).toBeTruthy();
    expect(screen.queryByText(copy.empty.title)).toBeNull();
  });

  it('still fails loudly when the report is unavailable for another reason', async () => {
    vi.mocked(getSellerReport).mockRejectedValueOnce(new ApiError('FORBIDDEN', 403));

    await expect(ReportsPage({ searchParams: Promise.resolve({}) })).rejects.toThrow();
  });

  it('shows the filter with an inline message when the API rejects the selection', async () => {
    vi.mocked(getSellerReport).mockRejectedValueOnce(new ApiError('INVALID_INPUT', 422));

    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        {await ReportsPage({
          searchParams: Promise.resolve({ from: '2025-01-01', to: '2026-01-01' }),
        })}
      </NextIntlClientProvider>,
    );

    expect(screen.getByText(copy.filters.invalid)).toBeTruthy();
    expect((screen.getByLabelText(copy.filters.from) as HTMLInputElement).value).toBe('2025-01-01');
    expect(screen.queryByRole('button', { name: copy.download.label })).toBeNull();
  });
});
