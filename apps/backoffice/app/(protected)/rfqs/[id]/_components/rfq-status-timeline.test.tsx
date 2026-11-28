import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it } from 'vitest';

import { RfqStatusTimeline } from '@/app/(protected)/rfqs/[id]/_components/rfq-status-timeline';
import type { QuoteSendTrackingResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

const SENT_AT = '2026-11-27T12:00:00Z';
const DELIVERY = {
  purpose: 'QUOTE',
  version_id: 'version',
  operation_id: 'operation',
  format: 'LINK',
  tracking_status: 'SENT',
  public_url: 'https://portal.example.test/quote',
  sent_at: SENT_AT,
  expires_at: null,
  created_at: SENT_AT,
} satisfies Omit<QuoteSendTrackingResponse, 'id' | 'channel' | 'destination'>;

it('distinguishes a portal publication from its WhatsApp delivery', () => {
  const detail: RfqDetailResponse = {
    rfq: {
      id: 'rfq',
      status: 'SENT',
      created_at: SENT_AT,
    } as RfqDetailResponse['rfq'],
    inbound_whatsapp_phone: null,
    inbound_email: null,
    quote: null,
    version: null,
    items: [],
    alternatives: {},
    rfq_status_history: [],
    quote_status_history: [],
    deliveries: [
      { ...DELIVERY, id: 'whatsapp', channel: 'WHATSAPP', destination: '+5491155550001' },
      { ...DELIVERY, id: 'portal', channel: 'WEBAPP', destination: 'customer-portal' },
    ],
  };
  const view = render(
    <NextIntlClientProvider
      locale="es-AR"
      timeZone="America/Argentina/Buenos_Aires"
      messages={messages}
    >
      <RfqStatusTimeline detail={detail} />
    </NextIntlClientProvider>,
  );

  const deliveries = view.getByRole('button', { name: /env.os\s*2$/i });
  fireEvent.click(deliveries);

  expect(view.getByText('WhatsApp', { exact: true })).toBeTruthy();
  expect(view.getByText('+5491155550001')).toBeTruthy();
  expect(view.getByText('Publicada en el portal')).toBeTruthy();
  expect(view.queryByText('Web', { exact: true })).toBeNull();
  expect(view.queryByText('customer-portal')).toBeNull();
});
