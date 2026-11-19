import { render, screen, waitFor } from '@testing-library/react';
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
  expect(await screen.findByText('lista.jpg')).toBeTruthy();
  expect(screen.getByText('audio.ogg')).toBeTruthy();
  expect(screen.getByText(messages.rfqs.originalRequest.files)).toBeTruthy();
  expect(screen.getByText(messages.rfqs.originalRequest.manualReview)).toBeTruthy();
  expect(fetcher).toHaveBeenCalledWith('/api/rfqs/rfq-1/attachments', {
    cache: 'no-store',
    headers: { 'X-Branch-Id': 'branch-1' },
  });
});

it('renders nothing when the order kept neither text nor files', async () => {
  const fetcher = stubAttachments([]);
  const { container } = renderCard('   ');
  await waitFor(() => expect(fetcher).toHaveBeenCalled());
  expect(container.textContent).toBe('');
});
