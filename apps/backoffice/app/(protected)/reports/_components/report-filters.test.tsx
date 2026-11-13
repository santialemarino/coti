import type { ComponentProps } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
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
const range = messages.common.dateRange;

function renderFilters(from = '2026-10-01', to = '2026-10-25') {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReportFilters from={from} to={to} />
    </NextIntlClientProvider>,
  );
  return view.container.querySelector('form')!;
}

function period() {
  return screen.getByRole('combobox', { name: copy.period });
}

// Picks a range through the calendar the way a seller does: the start, then the end.
function pickRange(fromLabel: string, toLabel: string) {
  fireEvent.click(period());
  const panel = screen.getByRole('dialog');
  const day = (label: string) =>
    within(panel).getByRole('button', { name: new RegExp(`(^|, )${label}(,|$)`) });
  fireEvent.click(day(fromLabel));
  fireEvent.click(day(toLabel));
}

describe('ReportFilters', () => {
  it('shows the period as one Argentine range', () => {
    renderFilters();

    expect(period().textContent).toBe('01/10/2026 – 25/10/2026');
  });

  // A URL can still carry a reversed period; the form refuses to send it on.
  it('blocks a range that runs backwards before it reaches the API', () => {
    const form = renderFilters('2026-10-20', '2026-10-10');
    fireEvent.submit(form);

    expect(screen.getByRole('alert').textContent).toBe(copy.invalidRange);
    expect(period().getAttribute('aria-invalid')).toBe('true');
  });

  it('clears the error once a range is picked again', () => {
    const form = renderFilters('2026-10-20', '2026-10-10');
    fireEvent.submit(form);
    pickRange('5 de octubre de 2026', '12 de octubre de 2026');

    expect(screen.queryByRole('alert')).toBeNull();
    expect(period().hasAttribute('aria-invalid')).toBe(false);
  });

  it('offers the frequent periods beside the calendar', () => {
    renderFilters();
    fireEvent.click(period());

    expect(
      within(screen.getByRole('group', { name: range.presetsLabel })).getAllByRole('button'),
    ).toHaveLength(Object.keys(range.presets).length);
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
  it('submits the picked period, and reports the navigation; none for a range it blocks', () => {
    begin.mockClear();
    const form = renderFilters('2026-10-20', '2026-10-10');
    fireEvent.submit(form);
    expect(begin).not.toHaveBeenCalled();

    pickRange('20 de octubre de 2026', '25 de octubre de 2026');
    fireEvent.submit(form);
    expect(begin).toHaveBeenCalledWith('/reports?from=2026-10-20&to=2026-10-25');
  });
});
