import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';
import { InvoicingSwitchCard } from './invoicing-switch-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/settings/invoicing/actions', () => ({ setInvoicingEnabled: vi.fn() }));

const { toast } = await import('sonner');
const { setInvoicingEnabled } = await import('@/app/(protected)/settings/invoicing/actions');

const copy = messages.invoicing.settings.switch;

function renderCard(enabled: boolean) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <InvoicingSwitchCard enabled={enabled} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('InvoicingSwitchCard', () => {
  it('names the switch and reflects the account', () => {
    renderCard(true);

    const toggle = screen.getByRole('switch', { name: copy.label });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(screen.getByText(copy.description)).toBeTruthy();
  });

  // Turning it on connects nothing, so there is nothing to confirm; the toast asks for a connection
  // only when there is none to take up again.
  it.each([
    [false, copy.enabled],
    [true, copy.enabledConnected],
  ])('turns invoicing on at once (connected: %s)', async (arcaConnected, toastCopy) => {
    vi.mocked(setInvoicingEnabled).mockResolvedValue({ hasInvoices: false, arcaConnected });
    renderCard(false);

    await act(async () => fireEvent.click(screen.getByRole('switch', { name: copy.label })));

    expect(setInvoicingEnabled).toHaveBeenCalledWith(true);
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(toastCopy));
  });

  // Off hides invoicing everywhere, so it asks first and writes only on confirmation. Comprobantes is
  // named only when it holds something.
  it.each([
    [false, copy.disabled],
    [true, copy.disabledWithInvoices],
  ])('asks before turning invoicing off (invoices: %s)', async (hasInvoices, toastCopy) => {
    vi.mocked(setInvoicingEnabled).mockResolvedValue({ hasInvoices, arcaConnected: false });
    renderCard(true);

    fireEvent.click(screen.getByRole('switch', { name: copy.label }));

    const dialog = await screen.findByRole('dialog');
    expect(dialog.textContent).toContain(copy.confirm.description);
    expect(setInvoicingEnabled).not.toHaveBeenCalled();

    await act(async () =>
      fireEvent.click(screen.getByRole('button', { name: copy.confirm.action })),
    );

    expect(setInvoicingEnabled).toHaveBeenCalledWith(false);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(toastCopy));
  });

  it('writes nothing when the confirmation is cancelled', async () => {
    renderCard(true);

    fireEvent.click(screen.getByRole('switch', { name: copy.label }));
    fireEvent.click(await screen.findByRole('button', { name: copy.confirm.cancel }));

    expect(setInvoicingEnabled).not.toHaveBeenCalled();
    expect(screen.getByRole('switch', { name: copy.label }).getAttribute('aria-checked')).toBe(
      'true',
    );
  });

  // An invoice ARCA has not answered keeps it on, and the message says why and where to look.
  it('words a refusal while an invoice is pending', async () => {
    vi.mocked(setInvoicingEnabled).mockResolvedValue({ error: 'INVOICE_PENDING' });
    renderCard(true);

    fireEvent.click(screen.getByRole('switch', { name: copy.label }));
    await act(async () =>
      fireEvent.click(await screen.findByRole('button', { name: copy.confirm.action })),
    );

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.INVOICE_PENDING));
    expect(toast.success).not.toHaveBeenCalled();
  });
});
