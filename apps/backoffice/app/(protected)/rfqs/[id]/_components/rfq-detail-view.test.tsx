import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider, useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import type { QuoteItemResponse, RfqDetailResponse, RfqRecord } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { RfqChangeDiff } from './rfq-change-diff';
import { RfqDetailView } from './rfq-detail-view';
import { SendQuoteDialog } from './send-quote-dialog';

const router = vi.hoisted(() => ({
  refresh: vi.fn(),
  push: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => router,
}));

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({
  fetchRfqDetail: vi.fn(),
  generateQuote: vi.fn(),
}));
// The send button's presence per status is what these tests exercise; the edit surface, diff and
// header are stubbed out, with the send dialog and the diff left as spies.
vi.mock('./rfq-items-table', () => ({ RfqItemsTable: () => null }));
vi.mock('./rfq-detail-header', () => ({ RfqDetailHeader: () => null }));
vi.mock('./send-quote-dialog', () => ({ SendQuoteDialog: vi.fn(() => null) }));
vi.mock('./rfq-change-diff', () => ({ RfqChangeDiff: vi.fn(() => null) }));

const { toast } = await import('sonner');
const { generateQuote } = await import('@/lib/api/rfqs-client');

const copy = messages.rfqs;

const BASE_TIME = '2026-09-07T20:45:00.000Z';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const RFQ_ID = '10000000-0000-4000-8000-000000000002';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const VERSION_ID = '30000000-0000-4000-8000-000000000002';
const SELLER_ID = '50000000-0000-4000-8000-000000000001';

const DRAFT_ITEM: QuoteItemResponse = {
  id: '40000000-0000-4000-8000-000000000001',
  version_id: VERSION_ID,
  product_id: '60000000-0000-4000-8000-000000000001',
  product_code: 'LAD-HUE-12',
  product_name: 'Ladrillo hueco 12x18x33',
  product_unit: 'unidad',
  requested_description: 'ladrillos huecos del 12',
  quantity: '500.00',
  unit: 'unidad',
  unit_price_snapshot: null,
  min_price_snapshot: null,
  subtotal: null,
  confidence_score: '0.9000',
  match_status: 'MATCHED',
  alternatives: [],
  pricing_unavailable: null,
  quantity_rationale: null,
  created_at: BASE_TIME,
};

const PRICED_ITEM: QuoteItemResponse = {
  ...DRAFT_ITEM,
  unit_price_snapshot: '780.00',
  min_price_snapshot: '700.00',
  subtotal: '390000.00',
  confidence_score: null,
  pricing_unavailable: false,
};

function draftRecord(): RfqRecord {
  return {
    id: RFQ_ID,
    quoteNumber: 1,
    client: 'Constructora',
    createdAt: BASE_TIME,
    channel: 'manual_entry',
    seller: 'Admin Dev',
    sellerId: SELLER_ID,
    branch: 'Villa Bosch',
    branchId: BRANCH_ID,
    itemCount: 1,
    status: 'GENERATED',
    needsFollowup: false,
  };
}

function makeDetail(quoteStatus: string, rfqStatus: string = quoteStatus): RfqDetailResponse {
  const changesRequested =
    quoteStatus === 'CHANGE_REQUESTED'
      ? {
          reason: 'Precio alto',
          original: {
            items: [
              {
                description: 'Ladrillo hueco 12x18x33',
                quantity: '500.00',
                unit: 'unidad',
                unit_price: '780.00',
                changed: true,
                change_type: 'modified' as const,
              },
            ],
            discounts: [],
            total: '390000.00',
          },
          requested: {
            items: [
              {
                description: 'Ladrillo hueco 12x18x33',
                quantity: '500.00',
                unit: 'unidad',
                unit_price: '760.00',
                changed: true,
                change_type: 'modified' as const,
              },
            ],
            discounts: [],
            total: '380000.00',
          },
        }
      : undefined;

  const draft = quoteStatus === 'DRAFT';

  return {
    rfq: {
      id: RFQ_ID,
      quote_number: 1,
      client: 'Constructora',
      created_at: BASE_TIME,
      channel: 'manual_entry',
      seller_id: SELLER_ID,
      seller: 'Admin Dev',
      branch: 'Villa Bosch',
      branch_id: BRANCH_ID,
      item_count: 1,
      total: draft ? null : '390000.00',
      status: rfqStatus,
      needs_followup: false,
      archived_at: null,
    },
    quote: {
      id: QUOTE_ID,
      branch_id: BRANCH_ID,
      client_id: null,
      rfq_id: RFQ_ID,
      seller_id: SELLER_ID,
      current_version_id: VERSION_ID,
      current_status: quoteStatus,
      expires_at: null,
      archived_at: null,
      needs_followup: false,
      followup_flagged_at: null,
      created_at: BASE_TIME,
      updated_at: BASE_TIME,
    },
    version: {
      id: VERSION_ID,
      quote_id: QUOTE_ID,
      author_id: SELLER_ID,
      version_number: 1,
      total: draft ? '0.00' : '390000.00',
      is_immutable: false,
      comment: null,
      created_at: BASE_TIME,
    },
    items: [draft ? DRAFT_ITEM : PRICED_ITEM],
    alternatives: {},
    rfq_status_history: [],
    quote_status_history: [],
    deliveries: [],
    discounts: [],
    changes_requested: changesRequested,
  };
}

function pricedResponse() {
  const detail = makeDetail('DRAFT', 'GENERATED');

  return {
    quote: {
      ...detail.quote!,
      current_status: 'QUOTED',
      updated_at: '2026-09-07T20:46:00.000Z',
    },
    version: {
      ...detail.version!,
      total: '390000.00',
    },
    items: [PRICED_ITEM],
  };
}

function RecordProbe() {
  const { records } = useRfqList();
  const record = records.find((candidate) => candidate.id === RFQ_ID);

  return (
    <div>
      <output aria-label="record-status">{record?.status}</output>
      <output aria-label="record-total">{record?.total}</output>
      <output aria-label="record-items">{record?.itemCount}</output>
    </div>
  );
}

function renderView(detail: RfqDetailResponse, activeBranchId: string | null = BRANCH_ID) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqListProvider
        records={[draftRecord()]}
        activeBranchId={activeBranchId}
        userName="Admin"
        userId="u-admin"
        isAdmin={true}
      >
        <RfqDetailView detail={detail} />
        <RecordProbe />
      </RfqListProvider>
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

describe('RfqDetailView tracking', () => {
  it('reports the empty delivery history once', () => {
    const view = renderView(makeDetail('QUOTED'));

    expect(view.getAllByText(copy.detail.timeline.noDeliveries)).toHaveLength(1);
  });
});

describe('RfqDetailView send flow', () => {
  const SendDialog = vi.mocked(SendQuoteDialog);
  const ChangeDiff = vi.mocked(RfqChangeDiff);

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

describe('RfqDetailView quote generation', () => {
  it('updates the shared RFQ list record when the draft reaches QUOTED', async () => {
    vi.mocked(generateQuote).mockResolvedValue(pricedResponse());

    const view = renderView(makeDetail('DRAFT', 'GENERATED'));

    fireEvent.click(view.getByRole('button', { name: copy.detail.items.generate }));

    await vi.waitFor(() => expect(view.getByLabelText('record-status').textContent).toBe('QUOTED'));
    expect(view.getByLabelText('record-total').textContent).toBe('390000.00');
    expect(view.getByLabelText('record-items').textContent).toBe('1');
    expect(router.refresh).toHaveBeenCalled();
  });

  it('names the missing branch before calling the API', () => {
    renderView(makeDetail('DRAFT', 'GENERATED'), null);

    fireEvent.click(screen.getByRole('button', { name: copy.detail.items.generate }));

    expect(generateQuote).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalledWith(copy.detail.items.toast.branchRequired);
  });
});
