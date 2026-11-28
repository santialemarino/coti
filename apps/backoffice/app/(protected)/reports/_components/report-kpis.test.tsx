import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it } from 'vitest';

import { ReportKpis } from '@/app/(protected)/reports/_components/report-kpis';
import type { ReportComparison, SellerReport } from '@/lib/api/reports';
import { createFormatters } from '@/lib/i18n/create-formatters';
import messages from '@/translations/es.json';

it('shows previous values, undefined bases and percentage points without infinity', () => {
  const kpis: SellerReport['kpis'] = {
    rfqsReceived: 120,
    quotesSent: 1,
    quotesAccepted: 0,
    sentAccepted: 0,
    conversionRate: 0.3,
    acceptedAmount: null,
    acceptedCurrency: null,
    acceptedAmounts: [],
    responseTime: { averageSeconds: null, medianSeconds: null, sample: 0 },
    timeToAcceptance: { averageSeconds: null, medianSeconds: null, sample: 0 },
    followUpPending: 0,
  };
  const comparison = {
    range: { from: '2026-11-03', to: '2026-11-09' },
    metrics: {
      rfqsReceived: { previous: '100', delta: '20', percentage: '20', percentagePoints: false },
      quotesSent: { previous: '0', delta: '1', percentage: null, percentagePoints: false },
      conversionRate: { previous: '0.2', delta: '10', percentage: null, percentagePoints: true },
    },
  } as ReportComparison;
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReportKpis kpis={kpis} comparison={comparison} fmt={createFormatters('es-AR')} />
    </NextIntlClientProvider>,
  );
  expect(view.getByText('Anterior: 100')).toBeTruthy();
  expect(view.getByText(/Variación: \+20.*\+20%/)).toBeTruthy();
  expect(view.getByText(/Sin base de comparación/)).toBeTruthy();
  expect(view.getByText(/\+10 pp/)).toBeTruthy();
  expect(view.container.textContent).not.toMatch(/NaN|Infinity/);
});

it('shows each accepted currency without a mixed total or an implicit ARS label', () => {
  const kpis: SellerReport['kpis'] = {
    rfqsReceived: 2,
    quotesSent: 2,
    quotesAccepted: 2,
    sentAccepted: 2,
    conversionRate: 1,
    acceptedAmount: null,
    acceptedCurrency: null,
    acceptedAmounts: [
      { currency: 'ARS', amount: '100.00' },
      { currency: 'USD', amount: '10.00' },
    ],
    responseTime: { averageSeconds: null, medianSeconds: null, sample: 0 },
    timeToAcceptance: { averageSeconds: null, medianSeconds: null, sample: 0 },
    followUpPending: 0,
  };
  const fmt = createFormatters('es-AR');
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReportKpis kpis={kpis} fmt={fmt} />
    </NextIntlClientProvider>,
  );
  expect(view.getByText(/ARS\s+\$\s*100,00/)).toBeTruthy();
  expect(view.getByText(/USD\s+US\$\s*10,00/)).toBeTruthy();
  expect(view.container.textContent).not.toContain(fmt.currency('110.00', 'ARS'));
});
