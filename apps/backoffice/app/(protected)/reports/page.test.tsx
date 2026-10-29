import { render, screen } from '@testing-library/react';
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
  })),
}));
vi.mock('@/lib/api/reports', () => ({ getSellerReport: vi.fn() }));

const { getSellerReport } = await import('@/lib/api/reports');
const { default: ReportsPage } = await import('@/app/(protected)/reports/page');

const copy = messages.reports;

function report(overrides: Partial<SellerReport> = {}): SellerReport {
  return {
    range: { from: '2026-09-26', to: '2026-10-25' },
    scope: { multiBranch: false, branchId: 'branch-1' },
    kpis: {
      rfqsReceived: 12,
      quotesSent: 4,
      quotesAccepted: 2,
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
      { date: '2026-09-26', sent: 1, accepted: 0 },
      { date: '2026-09-27', sent: 2, accepted: 1 },
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

async function renderReport(data: SellerReport, searchParams: { from?: string; to?: string } = {}) {
  vi.mocked(getSellerReport).mockResolvedValue(data);
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      {await ReportsPage({ searchParams: Promise.resolve(searchParams) })}
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ReportsPage', () => {
  it('shows the period figures and limits the report to the selected branch', async () => {
    await renderReport(report());

    expect(screen.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
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

    expect(getSellerReport).toHaveBeenCalledWith('2026-10-01', '2026-10-02');
    expect((screen.getByLabelText(copy.filters.from) as HTMLInputElement).value).toBe('2026-09-26');
    expect((screen.getByLabelText(copy.filters.to) as HTMLInputElement).value).toBe('2026-10-25');
  });

  it('says when the period has no activity instead of showing zeroes as data', async () => {
    await renderReport(
      report({
        kpis: {
          rfqsReceived: 0,
          quotesSent: 0,
          quotesAccepted: 0,
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

  it('states accepted and sent events without dividing by zero', async () => {
    const base = report();
    await renderReport(report({ kpis: { ...base.kpis, quotesSent: 0, quotesAccepted: 2 } }));

    expect(screen.getByText('2 aceptadas · 0 enviadas')).toBeTruthy();
  });

  it('shows the status section when quotes were created but no order arrived in the period', async () => {
    await renderReport(
      report({
        kpis: {
          rfqsReceived: 0,
          quotesSent: 0,
          quotesAccepted: 0,
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

  it('shows the filter with an inline message when the API rejects the period', async () => {
    vi.mocked(getSellerReport).mockRejectedValueOnce(new ApiError('INVALID_INPUT', 400));

    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        {await ReportsPage({
          searchParams: Promise.resolve({ from: '2025-01-01', to: '2026-01-01' }),
        })}
      </NextIntlClientProvider>,
    );

    expect(screen.getByText(copy.filters.invalidPeriod)).toBeTruthy();
    expect((screen.getByLabelText(copy.filters.from) as HTMLInputElement).value).toBe('2025-01-01');
  });
});
