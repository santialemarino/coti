import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { QuoteSendTrackingResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { QuoteDeliveryCard } from './quote-delivery-card';
import { SendQuoteDialog } from './send-quote-dialog';

vi.mock('./send-quote-dialog', () => ({ SendQuoteDialog: vi.fn(() => null) }));

const writeText = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));

const copy = messages.rfqs.detail.quoteCard;

const BASE_TIME = '2026-09-07T20:45:00.000Z';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const RFQ_ID = '10000000-0000-4000-8000-000000000002';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const VERSION_ID = '30000000-0000-4000-8000-000000000002';
const SELLER_ID = '50000000-0000-4000-8000-000000000001';
const PUBLIC_URL = 'https://coti.app/quotes/wa-token-a';

function tracking(overrides: Partial<QuoteSendTrackingResponse>): QuoteSendTrackingResponse {
  return {
    id: 'd-1',
    version_id: VERSION_ID,
    channel: 'whatsapp',
    destination: '+5491155550101',
    format: 'link',
    tracking_status: 'SENT',
    public_url: PUBLIC_URL,
    sent_at: BASE_TIME,
    expires_at: null,
    created_at: BASE_TIME,
    ...overrides,
  };
}

interface MakeDetailOptions {
  quoteStatus?: string;
  rfqStatus?: string;
  quoteNumber?: number | null;
  archived?: boolean;
  withQuote?: boolean;
  withVersion?: boolean;
  deliveries?: QuoteSendTrackingResponse[];
}

function makeDetail({
  quoteStatus = 'QUOTED',
  rfqStatus = quoteStatus,
  quoteNumber = 7,
  archived = false,
  withQuote = true,
  withVersion = true,
  deliveries = [],
}: MakeDetailOptions = {}): RfqDetailResponse {
  return {
    rfq: {
      id: RFQ_ID,
      quote_id: withQuote ? QUOTE_ID : null,
      quote_number: quoteNumber,
      client: 'Constructora',
      created_at: BASE_TIME,
      channel: 'manual_entry',
      seller_id: SELLER_ID,
      seller: 'Admin Dev',
      branch: 'Villa Bosch',
      branch_id: BRANCH_ID,
      item_count: 1,
      review_count: 0,
      total: '390000.00',
      status: rfqStatus,
      needs_followup: false,
      archived_at: archived ? BASE_TIME : null,
    },
    quote: withQuote
      ? {
          id: QUOTE_ID,
          branch_id: BRANCH_ID,
          client_id: null,
          rfq_id: RFQ_ID,
          seller_id: SELLER_ID,
          current_version_id: VERSION_ID,
          current_status: quoteStatus,
          expires_at: null,
          archived_at: archived ? BASE_TIME : null,
          needs_followup: false,
          followup_flagged_at: null,
          created_at: BASE_TIME,
          updated_at: BASE_TIME,
        }
      : null,
    version: withVersion
      ? {
          id: VERSION_ID,
          quote_id: QUOTE_ID,
          author_id: SELLER_ID,
          version_number: 1,
          total: '390000.00',
          is_immutable: false,
          comment: null,
          created_at: BASE_TIME,
        }
      : null,
    items: [],
    alternatives: {},
    rfq_status_history: [],
    quote_status_history: [],
    deliveries,
    discounts: [],
  };
}

function renderCard(detail: RfqDetailResponse) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <QuoteDeliveryCard detail={detail} onSent={vi.fn().mockResolvedValue(undefined)} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
});

afterEach(() => {
  delete (navigator as { clipboard?: unknown }).clipboard;
});

describe('QuoteDeliveryCard rendering', () => {
  it('renders nothing while the pedido has no quote or version yet', () => {
    const fromList = renderCard(makeDetail({ withQuote: false }));
    expect(fromList.container.firstChild).toBeNull();

    const noVersion = renderCard(makeDetail({ withVersion: false }));
    expect(noVersion.container.firstChild).toBeNull();
  });

  it('shows the reference, the version and the link the client received', () => {
    const view = renderCard(makeDetail({ deliveries: [tracking({}), tracking({ id: 'older-' })] }));

    expect(view.getByText('Cotización #07')).toBeTruthy();
    expect(view.getByText('Versión 1')).toBeTruthy();
    expect(view.getByText(PUBLIC_URL)).toBeTruthy();

    const link = view.getByRole('link', { name: copy.openQuote });
    expect(link.getAttribute('href')).toBe(PUBLIC_URL);
    expect(link.getAttribute('target')).toBe('_blank');
  });

  it('falls back to the draft copy on the badge while the quote is still internal', () => {
    const view = renderCard(makeDetail({ quoteStatus: 'DRAFT', rfqStatus: 'GENERATED' }));

    expect(view.getByText(messages.rfqs.status.GENERATED)).toBeTruthy();
  });

  it('shows the pending-number placeholder when no quote number exists yet', () => {
    const view = renderCard(makeDetail({ quoteNumber: null }));

    expect(view.getByText(`Cotización ${copy.numberPending}`)).toBeTruthy();
  });
});

describe('QuoteDeliveryCard live link', () => {
  it('copies the link and confirms inline', async () => {
    const view = renderCard(makeDetail({ deliveries: [tracking({})] }));

    fireEvent.click(view.getByRole('button', { name: copy.copyLink }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(PUBLIC_URL));
    expect(view.getByText(copy.copiedLink)).toBeTruthy();
  });

  it('ignores failed and empty-url attempts and shows the newest successful link', () => {
    const view = renderCard(
      makeDetail({
        deliveries: [
          tracking({
            id: 'failed-',
            tracking_status: 'FAILED',
            public_url: 'https://coti.app/quotes/x',
          }),
          tracking({
            id: 'pending-',
            tracking_status: 'PENDING',
            public_url: 'https://coti.app/quotes/y',
          }),
          tracking({ id: 'no-url-', tracking_status: 'SENT', public_url: '' }),
          tracking({
            id: 'live-',
            tracking_status: 'DELIVERED',
            public_url: 'https://coti.app/quotes/z',
          }),
        ],
      }),
    );

    expect(view.getByText('https://coti.app/quotes/z')).toBeTruthy();
    expect(view.queryByText(PUBLIC_URL)).toBeNull();
  });

  it('tells the seller the link is not available while nothing reached the client', () => {
    const view = renderCard(
      makeDetail({ deliveries: [tracking({ tracking_status: 'FAILED', public_url: '' })] }),
    );

    expect(view.getByText(copy.unavailable)).toBeTruthy();
    expect(view.queryByRole('link', { name: copy.openQuote })).toBeNull();
    expect(view.queryByRole('button', { name: copy.copyLink })).toBeNull();
  });
});

describe('QuoteDeliveryCard send action', () => {
  const SendDialog = vi.mocked(SendQuoteDialog);

  it('mounts the shared send dialog when the version is priced and review-ready', () => {
    for (const status of ['QUOTED']) {
      vi.clearAllMocks();
      renderCard(makeDetail({ quoteStatus: status }));

      expect(SendDialog, `send dialog surfaced on ${status}`).toHaveBeenCalledWith(
        expect.objectContaining({
          detail: expect.objectContaining({
            quote: expect.objectContaining({ current_status: status }),
          }),
        }),
        undefined,
      );
    }
  });

  it('keeps the dialog hidden unless the quote is review-ready', () => {
    for (const status of [
      'DRAFT',
      'GENERATED',
      'CHANGE_REQUESTED',
      'SENT',
      'ACCEPTED',
      'REJECTED',
    ]) {
      vi.clearAllMocks();
      renderCard(makeDetail({ quoteStatus: status }));

      expect(SendDialog, `send dialog surfaced on ${status}`).not.toHaveBeenCalled();
    }
  });

  it('keeps send and lifecycle actions hidden while the quote is archived', () => {
    const quoted = renderCard(makeDetail({ quoteStatus: 'QUOTED', archived: true }));
    expect(SendDialog).not.toHaveBeenCalled();
    expect(
      quoted.queryByRole('button', { name: messages.rfqs.detail.lifecycle.reactivate.button }),
    ).toBeNull();

    const accepted = renderCard(makeDetail({ quoteStatus: 'ACCEPTED', archived: true }));
    expect(
      accepted.queryByRole('button', { name: messages.rfqs.detail.lifecycle.reactivate.button }),
    ).toBeNull();
  });
});
