import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { NonOrderMessages } from '@/app/(protected)/rfqs/_components/non-order-messages';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => router }));

const copy = messages.rfqs.nonOrders;

const GREETING = {
  id: 'm1',
  channel_id: 'c1',
  kind: 'GREETING',
  sender_id: '5491155550201',
  sender_label: 'Juan Pérez',
  body: 'Hola, necesito cotizar materiales para una obra.',
  received_at: '2026-11-28T12:00:00.000Z',
};

function respond(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }));
}

function renderMessages(onOrderCreated = vi.fn(async () => undefined)) {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <NonOrderMessages branchId="b1" refreshKey={0} onOrderCreated={onOrderCreated} />
    </NextIntlClientProvider>,
  );
  return onOrderCreated;
}

async function open() {
  fireEvent.click(await screen.findByRole('button', { name: copy.show }));
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe('NonOrderMessages', () => {
  it('shows nothing when every message opened an order', async () => {
    const fetchMock = vi.fn(() => respond({ messages: [] }));
    vi.stubGlobal('fetch', fetchMock);
    renderMessages();

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByText(copy.title)).toBeNull();
  });

  it('lists a greeting folded until shown, with who sent it and its kind', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => respond({ messages: [GREETING] })),
    );
    renderMessages();

    expect(await screen.findByText(copy.title)).toBeTruthy();
    expect(screen.queryByText(GREETING.body)).toBeNull();
    await open();
    expect(screen.getByText(GREETING.body)).toBeTruthy();
    expect(screen.getByText('Juan Pérez')).toBeTruthy();
    expect(screen.getByText(copy.kind.GREETING)).toBeTruthy();
  });

  it('puts a message away and drops it from the list', async () => {
    const fetchMock = vi.fn((url: string) =>
      url.endsWith('/dismiss')
        ? Promise.resolve(new Response(null, { status: 204 }))
        : respond({ messages: [GREETING] }),
    );
    vi.stubGlobal('fetch', fetchMock);
    renderMessages();
    await open();

    fireEvent.click(screen.getByRole('button', { name: copy.dismiss }));
    await waitFor(() => expect(screen.queryByText(copy.title)).toBeNull());
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/inbound-messages/m1/dismiss',
      expect.objectContaining({ method: 'POST', headers: { 'X-Branch-Id': 'b1' } }),
    );
  });

  it('opens the order a seller asks for and goes to it', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        url.endsWith('/order')
          ? respond(
              { rfq: { id: 'r9', status: 'FAILED' }, quote: null, version: null, items: [] },
              201,
            )
          : respond({ messages: [GREETING] }),
      ),
    );
    const onOrderCreated = renderMessages();
    await open();

    fireEvent.click(screen.getByRole('button', { name: copy.createOrder }));
    await waitFor(() =>
      expect(router.push).toHaveBeenCalledWith(ROUTES.rfqsDetail('r9'), undefined),
    );
    expect(onOrderCreated).toHaveBeenCalled();
  });
});
