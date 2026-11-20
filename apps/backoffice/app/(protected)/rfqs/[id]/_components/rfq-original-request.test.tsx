import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, expect, it, vi } from 'vitest';

import { RfqOriginalRequest } from '@/app/(protected)/rfqs/[id]/_components/rfq-original-request';
import messages from '@/translations/es.json';

afterEach(() => vi.unstubAllGlobals());

function stubAttachments(attachments: unknown[]) {
  const fetcher = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify({ attachments }), { status: 200 }));
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

function renderCard(text?: string) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqOriginalRequest rfqId="rfq-1" branchId="branch-1" text={text} />
    </NextIntlClientProvider>,
  );
}

it('shows the order text as it arrived', async () => {
  const fetcher = stubAttachments([]);
  renderCard('Hola! Necesito 30 bolsas de cemento\ny 2 m3 de arena');
  expect(screen.getByText(messages.rfqs.originalRequest.title)).toBeTruthy();
  expect(screen.queryByText(/30 bolsas de cemento/)).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: messages.rfqs.originalRequest.title }));
  expect(screen.getByText(/30 bolsas de cemento/).textContent).toContain('\ny 2 m3 de arena');
  await waitFor(() => expect(fetcher).toHaveBeenCalled());
  expect(screen.queryByText(messages.rfqs.originalRequest.files)).toBeNull();
});

it('lists the files any channel received and flags material that needs manual review', async () => {
  const fetcher = stubAttachments([
    {
      id: 'one',
      original_filename: 'lista.jpg',
      processing_status: 'DONE',
      url: 'https://storage.example/lista',
    },
    {
      id: 'two',
      original_filename: 'audio.ogg',
      processing_status: 'FAILED',
      url: 'https://storage.example/audio',
    },
  ]);
  renderCard();
  fireEvent.click(screen.getByRole('button', { name: messages.rfqs.originalRequest.title }));
  expect(await screen.findByText('lista.jpg')).toBeTruthy();
  expect(screen.getByText('audio.ogg')).toBeTruthy();
  expect(screen.getByText(messages.rfqs.originalRequest.manualReview)).toBeTruthy();
  expect(fetcher).toHaveBeenCalledWith('/api/rfqs/rfq-1/attachments', {
    cache: 'no-store',
    headers: { 'X-Branch-Id': 'branch-1' },
  });
});

it('keeps the collapsed source section when a legacy order has no saved content', async () => {
  const fetcher = stubAttachments([]);
  const { container } = renderCard('   ');
  expect(container.textContent).toBe(messages.rfqs.originalRequest.title);
  expect(fetcher).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: messages.rfqs.originalRequest.title }));
  expect(await screen.findByText(messages.rfqs.originalRequest.unavailable)).toBeTruthy();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('fetches fresh links every time the section opens and offers a retry only on failure', async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(new Response('{}', { status: 500 }))
    .mockResolvedValue(new Response(JSON.stringify({ attachments: [] }), { status: 200 }));
  vi.stubGlobal('fetch', fetcher);
  renderCard('30 bolsas');
  const trigger = screen.getByRole('button', { name: messages.rfqs.originalRequest.title });
  expect(screen.queryByRole('button', { name: 'Actualizar enlaces' })).toBeNull();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole('button', { name: messages.rfqs.originalRequest.retry }));
  await waitFor(() => expect(screen.queryByText(messages.rfqs.originalRequest.error)).toBeNull());
  fireEvent.click(trigger);
  fireEvent.click(trigger);
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3));
});
