import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqItemsTable } from '@/app/(protected)/rfqs/[id]/_components/rfq-items-table';
import { ApiError } from '@/lib/api/errors';
import type { QuoteItemResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock('@/lib/api/rfqs-client', () => ({
  addQuoteItem: vi.fn(),
  deleteQuoteItem: vi.fn(),
  updateQuoteItem: vi.fn(),
}));

const { toast } = await import('sonner');
const { updateQuoteItem } = await import('@/lib/api/rfqs-client');

const copy = messages.rfqs;

const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const QUOTE_ID = 'q0000000-0000-4000-8000-000000000001';
const ITEM_ID = 'i0000000-0000-4000-8000-000000000001';

const PRICED_ITEM: QuoteItemResponse = {
  id: ITEM_ID,
  version_id: 'v0000000-0000-4000-8000-000000000001',
  product_id: 'p0000000-0000-4000-8000-000000000001',
  product_code: 'LAD-HUE-12',
  product_name: 'Ladrillo hueco 12x18x33',
  product_unit: 'unidad',
  requested_description: 'ladrillos huecos del 12',
  quantity: '500.00',
  unit: 'unidad',
  unit_price_snapshot: '780.00',
  min_price_snapshot: '700.00',
  subtotal: '390000.00',
  confidence_score: '0.9000',
  match_status: 'MATCHED',
  alternatives: [],
  pricing_unavailable: false,
  quantity_rationale: null,
  created_at: '2026-09-07T20:45:00.000Z',
};

function renderItems(activeBranchId: string | null = BRANCH_ID) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqListProvider
        records={[]}
        activeBranchId={activeBranchId}
        userName="Admin"
        userId="u-admin"
        isAdmin={true}
      >
        <RfqItemsTable
          quoteId={QUOTE_ID}
          quoteStatus="QUOTED"
          items={[PRICED_ITEM]}
          discounts={[]}
          onItemsChange={vi.fn()}
        />
      </RfqListProvider>
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RfqItemsTable action errors', () => {
  it('shows the quote-state message instead of a generic toast', async () => {
    vi.mocked(updateQuoteItem).mockRejectedValue(new ApiError('QUOTE_NOT_DRAFT', 409));

    const view = renderItems();
    const price = view.getByDisplayValue('780,00');

    fireEvent.change(price, { target: { value: '900' } });
    fireEvent.blur(price);

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(copy.detail.items.errors.QUOTE_NOT_DRAFT),
    );
  });

  it('names the missing branch before attempting the write', async () => {
    const view = renderItems(null);
    const price = view.getByDisplayValue('780,00');

    fireEvent.change(price, { target: { value: '900' } });
    fireEvent.blur(price);

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(copy.detail.items.toast.branchRequired),
    );
    expect(updateQuoteItem).not.toHaveBeenCalled();
  });
});

describe('RfqItemsTable price input', () => {
  it('shows an implicit currency sign and sends an Argentine-formatted value as a decimal', async () => {
    vi.mocked(updateQuoteItem).mockResolvedValue({
      ...PRICED_ITEM,
      unit_price_snapshot: '132467.89',
    });

    const view = renderItems();
    const price = view.getByDisplayValue('780,00');
    expect(price.parentElement?.textContent).toContain('$');

    fireEvent.change(price, { target: { value: '132.467,89' } });
    fireEvent.blur(price);

    await vi.waitFor(() =>
      expect(updateQuoteItem).toHaveBeenCalledWith(QUOTE_ID, ITEM_ID, {
        unit_price_snapshot: '132467.89',
      }),
    );
  });
});
