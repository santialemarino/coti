import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ClientRegistrationLink } from '@/app/(protected)/clients/[id]/_components/client-registration-link';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/rfqs/_components/rfq-list-context', () => ({
  useRfqList: () => ({ activeBranchId: 'b1' }),
}));
const t = messages.clients.registration;

function refuse(status: number, code?: string) {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ error: 'refused', ...(code ? { code } : {}) }), { status }),
      ),
  );
}

async function send() {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientRegistrationLink clientId="c1" phone="+5491155550000" email="juan@example.test" />
    </NextIntlClientProvider>,
  );
  fireEvent.click(view.getByRole('button', { name: t.whatsapp }));
  fireEvent.click(await view.findByRole('button', { name: t.send }));
  await waitFor(() => expect(toast.error).toHaveBeenCalled());
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe('ClientRegistrationLink', () => {
  // WhatsApp refuses free text past its window, so the seller is pointed at the link they can copy.
  it('words a closed WhatsApp window and a missing customer site', async () => {
    refuse(422, 'WHATSAPP_WINDOW_CLOSED');
    await send();
    expect(toast.error).toHaveBeenCalledWith(messages.errors.WHATSAPP_WINDOW_CLOSED);

    vi.clearAllMocks();
    refuse(503, 'PORTAL_NOT_CONFIGURED');
    await send();
    expect(toast.error).toHaveBeenCalledWith(messages.errors.PORTAL_NOT_CONFIGURED);
  });

  it('keeps the channel advice for a delivery that failed', async () => {
    refuse(503, 'NOT_CONFIGURED');
    await send();
    expect(toast.error).toHaveBeenCalledWith(t.errors.NOT_CONFIGURED);
  });
});
