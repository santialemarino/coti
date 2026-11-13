import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { DataExportForm } from '@/app/(protected)/settings/export/_components/data-export-form';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const copy = messages.dataExport;
const BRANCHES = [
  { id: 'b1', name: 'Centro', isActive: true },
  { id: 'b2', name: 'Norte', isActive: false },
];

function renderForm(props: Partial<Parameters<typeof DataExportForm>[0]> = {}) {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <DataExportForm branches={BRANCHES} offersInvoices {...props} />
    </NextIntlClientProvider>,
  );
}

function period() {
  return screen.getByRole('combobox', { name: copy.period.label });
}

function day(label: string) {
  return within(screen.getByRole('dialog')).getByRole('button', {
    name: new RegExp(`(^|, )${label}(,|$)`),
  });
}

function exportResponse(filename: string) {
  return new Response('numero\r\n1\r\n', {
    status: 200,
    headers: {
      'Content-Type': 'text/csv; charset=utf-8',
      'Content-Disposition': `attachment; filename="${filename}"`,
    },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  // An empty period opens on the current month; only the clock is faked, so waits still run.
  vi.useFakeTimers({ now: new Date(2026, 10, 13), toFake: ['Date'] });
  vi.stubGlobal('URL', {
    ...URL,
    createObjectURL: vi.fn(() => 'blob:export'),
    revokeObjectURL: vi.fn(),
  });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('DataExportForm', () => {
  it('downloads quotes for the chosen period and names the file it saved', async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(exportResponse('cotizaciones-2026-11-05.csv'));
    vi.stubGlobal('fetch', fetchMock);
    renderForm();

    fireEvent.click(period());
    fireEvent.click(day('1 de noviembre de 2026'));
    fireEvent.click(day('5 de noviembre de 2026'));
    expect(period().textContent).toBe('01/11/2026 – 05/11/2026');
    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/exports/quotes?format=csv&from=2026-11-01&to=2026-11-05',
    );
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('Descargamos «cotizaciones-2026-11-05.csv».'),
    );
  });

  // Either end may stay open: an empty end exports from the start of history, or up to today.
  it('sends only the start of a period left open at the end', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(exportResponse('cotizaciones.csv'));
    vi.stubGlobal('fetch', fetchMock);
    renderForm();

    expect(period().textContent).toBe(copy.period.placeholder);
    fireEvent.click(period());
    fireEvent.click(day('1 de noviembre de 2026'));
    fireEvent.click(screen.getByRole('button', { name: messages.common.dateRange.done }));
    expect(period().textContent).toBe(`01/11/2026 – ${copy.period.openEnd}`);
    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/exports/quotes?format=csv&from=2026-11-01');
  });

  it('drops the period and the branch for clients, and offers XML', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(exportResponse('clientes.xml'));
    vi.stubGlobal('fetch', fetchMock);
    renderForm();

    fireEvent.click(screen.getByLabelText(copy.entity.clients.label));
    expect(screen.queryByRole('combobox', { name: copy.period.label })).toBeNull();
    expect(screen.queryByRole('combobox', { name: copy.branch.label })).toBeNull();
    fireEvent.click(screen.getByRole('radio', { name: copy.format.xml }));
    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/exports/clients?format=xml');
  });

  it('offers invoices only to an account that invoices, and a branch only among several', () => {
    renderForm({ offersInvoices: false, branches: [BRANCHES[0]!] });

    expect(screen.queryByLabelText(copy.entity.invoices.label)).toBeNull();
    expect(screen.queryByRole('combobox', { name: copy.branch.label })).toBeNull();
  });

  it('words a refusal from the API instead of failing silently', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response(JSON.stringify({ code: 'INTERNAL' }), { status: 500 })),
    );
    renderForm();

    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.download.error));
  });
});
