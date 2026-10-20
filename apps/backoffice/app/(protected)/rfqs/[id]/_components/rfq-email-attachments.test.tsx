import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, expect, it, vi } from 'vitest';

import { RfqEmailAttachments } from '@/app/(protected)/rfqs/[id]/_components/rfq-email-attachments';
import messages from '@/translations/es.json';

afterEach(() => vi.unstubAllGlobals());

it('shows original filenames and flags material that needs manual review', async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        attachments: [
          {
            id: 'one',
            original_filename: 'original.eml',
            processing_status: 'DONE',
            url: 'https://storage.example/original',
          },
          {
            id: 'two',
            original_filename: 'order.bin',
            processing_status: 'FAILED',
            url: 'https://storage.example/order',
          },
        ],
      }),
      { status: 200 },
    ),
  );
  vi.stubGlobal('fetch', fetcher);
  render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqEmailAttachments rfqId="rfq-1" branchId="branch-1" />
    </NextIntlClientProvider>,
  );
  expect(await screen.findByText('original.eml')).toBeTruthy();
  expect(screen.getByText('order.bin')).toBeTruthy();
  expect(screen.getByText(messages.rfqs.emailAttachments.manualReview)).toBeTruthy();
  expect(fetcher).toHaveBeenCalledWith('/api/rfqs/rfq-1/attachments', {
    cache: 'no-store',
    headers: { 'X-Branch-Id': 'branch-1' },
  });
});
