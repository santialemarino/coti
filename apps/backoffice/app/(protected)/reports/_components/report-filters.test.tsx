import type { ComponentProps } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { ReportFilters } from '@/app/(protected)/reports/_components/report-filters';
import messages from '@/translations/es.json';

// next/form needs the app router; a plain form keeps the submit and its action observable.
vi.mock('next/form', () => ({
  default: (props: ComponentProps<'form'>) => <form {...props} />,
}));

const begin = vi.hoisted(() => vi.fn(() => 1));
vi.mock('@/hooks/use-navigation-pending', () => ({ useNavigationPending: () => ({ begin }) }));

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

  it('submits every active filter and shows the seller selector to administrators', () => {
    const { container } = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <ReportFilters
          from="2026-10-01"
          to="2026-10-25"
          statuses={['SENT', 'ACCEPTED']}
          channel="EMAIL"
          sellerId="seller-1"
          clientId="client-1"
          sellers={[{ id: 'seller-1', name: 'Ana' }]}
          clients={[{ id: 'client-1', name: 'Obra Norte' }]}
          showSeller
        />
      </NextIntlClientProvider>,
    );

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
    expect(screen.getByRole('combobox', { name: copy.seller })).toBeTruthy();
  });

  it('lets an ordered range through to the reports route', () => {
    const form = renderFilters();
    const submit = new Event('submit', { bubbles: true, cancelable: true });
    form.dispatchEvent(submit);

    expect(submit.defaultPrevented).toBe(false);
    expect(form.getAttribute('action')).toBe('/reports');
    expect(screen.queryByRole('alert')).toBeNull();
  });

  // next/form navigates on its own, so the shell is told where it goes and shows the bar meanwhile.
  it('reports the navigation it submits, and none for a range it blocks', () => {
    begin.mockClear();
    const form = renderFilters();
    fireEvent.change(screen.getByLabelText(copy.from), { target: { value: '2026-10-20' } });
    fireEvent.change(screen.getByLabelText(copy.to), { target: { value: '2026-10-10' } });
    fireEvent.submit(form);
    expect(begin).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(copy.to), { target: { value: '2026-10-25' } });
    fireEvent.submit(form);
    expect(begin).toHaveBeenCalledWith('/reports?from=2026-10-20&to=2026-10-25');
  });
});
