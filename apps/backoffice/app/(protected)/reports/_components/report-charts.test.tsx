import { render } from '@testing-library/react';
import { expect, it } from 'vitest';

import { StatusPieChart, TrendChart } from '@/app/(protected)/reports/_components/report-charts';

const format = (value: number) => String(value);

// A table ignores sr-only's 1px width and widens the page on a phone; only a wrapper may hide it.
it('hides each accessible table inside a wrapper, never on the table itself', () => {
  const { container } = render(
    <>
      <StatusPieChart
        items={[{ key: 'SENT', label: 'Enviada', value: 3, tone: 'sent' }]}
        summary="3 cotizaciones"
        ariaLabel="Cotizaciones por estado"
        label="Estado"
        valueLabel="Cantidad"
        formatValue={format}
        formatShare={format}
      />
      <TrendChart
        points={[
          { date: '2026-11-01', rfqs: 2, sent: 1, accepted: 1, conversionRate: 1 },
          { date: '2026-11-02', rfqs: 1, sent: 1, accepted: 0, conversionRate: 0 },
        ]}
        labels={{
          rfqs: 'Pedidos',
          sent: 'Envíos',
          accepted: 'Aceptaciones',
          conversion: 'Conversión',
          bucket: 'Día',
        }}
        ariaLabel="Evolución"
        scrollLabel="Desplazá el gráfico"
        formatBucket={(date) => date}
        formatValue={format}
        formatShare={format}
      />
    </>,
  );

  const tables = [...container.querySelectorAll('table')];
  expect(tables).toHaveLength(2);
  for (const table of tables) {
    expect(table.classList.contains('sr-only')).toBe(false);
    expect(table.parentElement?.classList.contains('sr-only')).toBe(true);
  }
});
