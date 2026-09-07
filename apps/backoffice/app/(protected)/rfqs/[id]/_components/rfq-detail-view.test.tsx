import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { QuoteItemResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { RfqChangeDiff } from './rfq-change-diff';
import { RfqDetailView } from './rfq-detail-view';
import { SendQuoteDialog } from './send-quote-dialog';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ fetchRfqDetail: vi.fn(), generateQuote: vi.fn() }));
// The send button's presence per status is what these tests exercise; the edit surface, diff and
// chrome are stubbed out, with the send dialog and the diff left as spies so the test can assert
// the shared send flow mounts in CHANGE_REQUESTED too.
vi.mock('./rfq-items-table', () => ({ RfqItemsTable: () => null }));
vi.mock('./rfq-detail-header', () => ({ RfqDetailHeader: () => null }));
vi.mock('./rfq-status-timeline', () => ({ RfqStatusTimeline: () => null }));
vi.mock('./send-quote-dialog', () => ({ SendQuoteDialog: vi.fn(() => null) }));
vi.mock('./rfq-change-diff', () => ({ RfqChangeDiff: vi.fn(() => null) }));

const copy = messages.rfqs;

const SendDialog = vi.mocked(SendQuoteDialog);
const ChangeDiff = vi.mocked(RfqChangeDiff);

const RFQ_ID = '10000000-0000-4000-8000-000000000002';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const VERSION_ID = '30000000-0000-4000-8000-000000000002';

const ITEM: QuoteItemResponse = {
  id: '40000000-0000-4000-8000-000000000001',
  version_id: VERSION_ID,
  product_id: null,
  product_code: null,
  product_name: null,
  product_unit: null,
  requested_description: 'Cemento portland 50kg',
  quantity: '10',
  unit: 'u',
  unit_price_snapshot: '12340.50',
  min_price_snapshot: null,
  subtotal: '123405.00',
  confidence_score: null,
  match_status: 'MATCHED',
  alternatives: [],
  pricing_unavailable: false,
  quantity_rationale: null,
  created_at: '2026-08-06T10:00:00.000Z',
};

function makeDetail(quoteStatus: string, rfqStatus: string = quoteStatus): RfqDetailResponse {
  const changesRequested =
    quoteStatus === 'CHANGE_REQUESTED'
      ? {
          reason: 'Precio alto',
          original: {
            items: [
              {
                description: 'Cemento portland 50kg',
                quantity: '10',
                unit: 'u',
                unit_price: '12340.50',
                changed: true,
                change_type: 'modified' as const,
              },
            ],
            discounts: [],
            total: '123405.00',
          },
          requested: {
            items: [
              {
                description: 'Cemento portland 50kg',
                quantity: '10',
                unit: 'u',
                unit_price: '12000.00',
                changed: true,
                change_type: 'modified' as const,
              },
            ],
            discounts: [],
            total: '120000.00',
          },
        }
      : undefined;

  return {
    rfq: {
      id: RFQ_ID,
      client: 'Constructora A',
      created_at: '2026-08-06T10:00:00.000Z',
      channel: 'whatsapp',
      seller_id: null,
      seller: 'María López',
      branch: 'Centro',
      branch_id: 'b0000000-0000-4000-8000-000000000001',
      item_count: 1,
      total: '123405.00',
      status: rfqStatus,
      needs_followup: false,
      archived_at: null,
    },
    quote: {
      id: QUOTE_ID,
      branch_id: 'b0000000-0000-4000-8000-000000000001',
      client_id: null,
      rfq_id: RFQ_ID,
      seller_id: null,
      current_version_id: VERSION_ID,
      current_status: quoteStatus,
      expires_at: null,
      archived_at: null,
      needs_followup: false,
      followup_flagged_at: null,
      created_at: '2026-08-06T10:00:00.000Z',
      updated_at: '2026-08-06T10:00:00.000Z',
    },
    version: {
      id: VERSION_ID,
      quote_id: QUOTE_ID,
      author_id: null,
      version_number: 1,
      total: quoteStatus === 'CHANGE_REQUESTED' ? '120000.00' : '123405.00',
      is_immutable: false,
      comment: null,
      created_at: '2026-08-06T10:00:00.000Z',
    },
    items: [ITEM],
    alternatives: {},
    discounts: [],
    changes_requested: changesRequested,
  };
}

function renderView(detail: RfqDetailResponse) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqDetailView detail={detail} />
    </NextIntlClientProvider>,
  );
}

function dialogPropsFor(status: string) {
  return expect.objectContaining({
    detail: expect.objectContaining({
      quote: expect.objectContaining({ current_status: status }),
    }),
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RfqDetailView send flow', () => {
  it('mounts the shared send dialog on a QUOTED quote', () => {
    renderView(makeDetail('QUOTED'));

    expect(SendDialog).toHaveBeenCalledWith(dialogPropsFor('QUOTED'), undefined);
  });

  it('reuses the same send dialog on CHANGE_REQUESTED and keeps the diff', () => {
    renderView(makeDetail('CHANGE_REQUESTED'));

    expect(SendDialog).toHaveBeenCalledWith(dialogPropsFor('CHANGE_REQUESTED'), undefined);
    expect(ChangeDiff).toHaveBeenCalledTimes(1);
  });

  it('keeps the send button hidden until a quote is review-ready', () => {
    for (const status of ['SENT', 'ACCEPTED', 'REJECTED', 'GENERATED']) {
      vi.clearAllMocks();
      renderView(makeDetail(status));
      expect(SendDialog, `send dialog surfaced on ${status}`).not.toHaveBeenCalled();
    }
  });

  it('leaves the DRAFT generate action in place', () => {
    const view = renderView(makeDetail('DRAFT', 'GENERATED'));

    expect(view.getByRole('button', { name: copy.detail.items.generate })).toBeTruthy();
    expect(SendDialog).not.toHaveBeenCalled();
  });
});
