import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { InvoiceRecoveryButton } from '@/components/invoice-recovery-button';
import { ApiError } from '@/lib/api/errors';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ refresh }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/invoicing-client', () => ({ recoverInvoice: vi.fn() }));

const { toast } = await import('sonner');
const { recoverInvoice } = await import('@/lib/api/invoicing-client');

function press(onRecovered = vi.fn()) {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <InvoiceRecoveryButton invoiceId="inv-1" branchId="branch-1" onRecovered={onRecovered} />
    </NextIntlClientProvider>,
  );
  fireEvent.click(view.getByRole('button', { name: messages.invoicing.history.recover }));
  return onRecovered;
}

beforeEach(() => vi.clearAllMocks());

describe('InvoiceRecoveryButton', () => {
  it('asks about the invoice in its own branch and refreshes once it answers', async () => {
    vi.mocked(recoverInvoice).mockResolvedValue({} as never);
    const onRecovered = press();

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith(messages.invoicing.history.recovered),
    );
    expect(recoverInvoice).toHaveBeenCalledWith('inv-1', 'branch-1');
    expect(onRecovered).toHaveBeenCalled();
    expect(refresh).toHaveBeenCalled();
  });

  it('words a refusal by its code and leaves the screen as it was', async () => {
    vi.mocked(recoverInvoice).mockRejectedValue(new ApiError('INVOICE_IN_PROGRESS', 409));
    const onRecovered = press();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(messages.errors.INVOICE_IN_PROGRESS),
    );
    expect(onRecovered).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
  });
});
