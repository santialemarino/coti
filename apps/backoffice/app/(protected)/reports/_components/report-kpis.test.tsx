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
    acceptedAmount: '0.00',
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
