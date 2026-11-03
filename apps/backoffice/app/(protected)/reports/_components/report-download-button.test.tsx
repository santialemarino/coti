import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ReportDownloadButton } from '@/app/(protected)/reports/_components/report-download-button';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }));

const copy = messages.reports.download;

function renderButton() {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReportDownloadButton from="2026-10-01" to="2026-10-02" />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('URL', {
    ...URL,
    createObjectURL: vi.fn(() => 'blob:report'),
    revokeObjectURL: vi.fn(),
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ReportDownloadButton', () => {
  it('downloads the generated PDF with the server filename', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('%PDF-1.4', {
        status: 200,
        headers: {
          'Content-Type': 'application/pdf',
          'Content-Disposition': 'attachment; filename="reporte-2026-10-01-2026-10-02.pdf"',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);
    let download = '';
    let href = '';
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      download = this.download;
      href = this.href;
    });
    renderButton();

    fireEvent.click(screen.getByRole('button', { name: copy.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/reports/pdf?from=2026-10-01&to=2026-10-02');
    expect(download).toBe('reporte-2026-10-01-2026-10-02.pdf');
    expect(href).toBe('blob:report');
  });

  it('forwards the active filters to the PDF proxy', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('%PDF-1.4', {
        status: 200,
        headers: {
          'Content-Type': 'application/pdf',
          'Content-Disposition': 'attachment; filename="reporte.pdf"',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <ReportDownloadButton
          from="2026-10-01"
          to="2026-10-02"
          statuses={['SENT', 'ACCEPTED']}
          channel="EMAIL"
          sellerId="seller-1"
          clientId="client-1"
        />
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: copy.label }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/reports/pdf?from=2026-10-01&to=2026-10-02&status=SENT%2CACCEPTED&channel=EMAIL&seller_id=seller-1&client_id=client-1',
    );
  });

  it('reports a failed download without navigating away', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockResolvedValue(Response.json({ code: 'INTERNAL' }, { status: 500 })),
    );
    renderButton();

    fireEvent.click(screen.getByRole('button', { name: copy.label }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.error));
    expect(screen.getByRole('button', { name: copy.label })).toBeTruthy();
  });
});
