import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { ReportFilters } from '@/app/(protected)/reports/_components/report-filters';
import messages from '@/translations/es.json';

const copy = messages.reports.filters;

function renderFilters(from = '2026-10-01', to = '2026-10-25') {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReportFilters from={from} to={to} />
    </NextIntlClientProvider>,
  );
  return view.container.querySelector('form')!;
}

describe('ReportFilters', () => {
  it('blocks a range that runs backwards before it reaches the API', () => {
    const form = renderFilters();
    fireEvent.change(screen.getByLabelText(copy.from), { target: { value: '2026-10-20' } });
    fireEvent.change(screen.getByLabelText(copy.to), { target: { value: '2026-10-10' } });
    fireEvent.submit(form);

    expect(screen.getByRole('alert').textContent).toBe(copy.invalidRange);
    expect(screen.getByLabelText(copy.from).getAttribute('aria-invalid')).toBe('true');
  });

  it('clears the error once the dates are ordered again', () => {
    const form = renderFilters();
    fireEvent.change(screen.getByLabelText(copy.from), { target: { value: '2026-10-20' } });
    fireEvent.change(screen.getByLabelText(copy.to), { target: { value: '2026-10-10' } });
    fireEvent.submit(form);
    fireEvent.change(screen.getByLabelText(copy.to), { target: { value: '2026-10-25' } });

    expect(screen.queryByRole('alert')).toBeNull();
  });
});
