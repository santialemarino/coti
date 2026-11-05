import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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
  vi.stubGlobal('URL', {
    ...URL,
    createObjectURL: vi.fn(() => 'blob:export'),
    revokeObjectURL: vi.fn(),
  });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
});

afterEach(() => {
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

    fireEvent.change(screen.getByLabelText(copy.period.from), { target: { value: '2026-11-01' } });
    fireEvent.change(screen.getByLabelText(copy.period.to), { target: { value: '2026-11-05' } });
    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/exports/quotes?format=csv&from=2026-11-01&to=2026-11-05',
    );
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('Descargamos «cotizaciones-2026-11-05.csv».'),
    );
  });

  it('refuses a reversed period without calling the API', () => {
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal('fetch', fetchMock);
    renderForm();

    fireEvent.change(screen.getByLabelText(copy.period.from), { target: { value: '2026-11-05' } });
    fireEvent.change(screen.getByLabelText(copy.period.to), { target: { value: '2026-11-01' } });
    fireEvent.click(screen.getByRole('button', { name: copy.download.label }));

    expect(screen.getByRole('alert').textContent).toBe(copy.period.invalidRange);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('drops the period and the branch for clients, and offers XML', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(exportResponse('clientes.xml'));
    vi.stubGlobal('fetch', fetchMock);
    renderForm();

    fireEvent.click(screen.getByLabelText(copy.entity.clients.label));
    expect(screen.queryByLabelText(copy.period.from)).toBeNull();
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
