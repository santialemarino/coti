import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TooltipProvider } from '@repo/ui/components';
import { ClientNameEditor } from '@/app/(protected)/clients/[id]/_components/client-name-editor';
import { renameClient } from '@/lib/api/clients-client';
import { ApiError } from '@/lib/api/errors';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ refresh }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({ renameClient: vi.fn() }));
const t = messages.clients.rename;

function show(registered = false) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <TooltipProvider>
        <ClientNameEditor clientId="c1" name="🔨 Juanchi" registered={registered} />
      </TooltipProvider>
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('ClientNameEditor', () => {
  // A registered profile's name is its customer's, so there is nothing to press, only why.
  it('offers no rename on a registered profile and says who owns the name', () => {
    const view = show(true);

    expect(view.queryByRole('button', { name: t.edit })).toBeNull();
    expect(view.getByText(t.registered)).toBeTruthy();
  });

  it('saves the trimmed name of a provisional profile', async () => {
    vi.mocked(renameClient).mockResolvedValue({ name: 'Juan Pérez' } as never);
    const view = show();

    fireEvent.click(view.getByRole('button', { name: t.edit }));
    const input = await view.findByPlaceholderText(t.placeholder);
    fireEvent.change(input, { target: { value: '  Juan Pérez ' } });
    fireEvent.click(view.getByRole('button', { name: t.submit }));

    await waitFor(() => expect(renameClient).toHaveBeenCalledWith('c1', 'Juan Pérez'));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it('words a refusal from the API under the field', async () => {
    vi.mocked(renameClient).mockRejectedValue(new ApiError('CLIENT_REGISTERED', 403));
    const view = show();

    fireEvent.click(view.getByRole('button', { name: t.edit }));
    fireEvent.change(await view.findByPlaceholderText(t.placeholder), {
      target: { value: 'Otro nombre' },
    });
    fireEvent.click(view.getByRole('button', { name: t.submit }));

    expect(await view.findByText(messages.errors.CLIENT_REGISTERED)).toBeTruthy();
  });
});
