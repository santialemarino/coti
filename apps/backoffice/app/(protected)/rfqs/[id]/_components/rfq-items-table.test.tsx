import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqItemsTable } from '@/app/(protected)/rfqs/[id]/_components/rfq-items-table';
import { ApiError } from '@/lib/api/errors';
import type { QuoteItemResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock('@/lib/api/catalog', () => ({ searchCatalog: vi.fn().mockResolvedValue([]) }));

vi.mock('@/lib/api/rfqs-client', () => ({
  addQuoteItem: vi.fn(),
  deleteQuoteItem: vi.fn(),
  updateQuoteItem: vi.fn(),
}));

const { toast } = await import('sonner');
const { deleteQuoteItem, updateQuoteItem } = await import('@/lib/api/rfqs-client');

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
  confidence_level: 'HIGH',
  match_status: 'MATCHED',
  alternatives: [],
  pricing_unavailable: false,
  quantity_rationale: null,
  created_at: '2026-09-07T20:45:00.000Z',
};

function renderItems(
  activeBranchId: string | null = BRANCH_ID,
  quoteStatus = 'QUOTED',
  items: QuoteItemResponse[] = [PRICED_ITEM],
) {
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
          quoteStatus={quoteStatus}
          branchId={BRANCH_ID}
          items={items}
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

  it('scopes the write to the order\u2019s branch even with no branch selected in the header', async () => {
    vi.mocked(updateQuoteItem).mockResolvedValue({ ...PRICED_ITEM, unit_price_snapshot: '900.00' });

    const view = renderItems(null);
    const price = view.getByDisplayValue('780,00');

    fireEvent.change(price, { target: { value: '900' } });
    fireEvent.blur(price);

    await vi.waitFor(() =>
      expect(updateQuoteItem).toHaveBeenCalledWith(QUOTE_ID, ITEM_ID, BRANCH_ID, {
        unit_price_snapshot: '900',
      }),
    );
  });
});

describe('RfqItemsTable price input', () => {
  it('shows the change-request draft price without allowing a price edit', () => {
    const view = renderItems(BRANCH_ID, 'CHANGE_REQUESTED');

    expect(
      view.getByRole('columnheader', { name: copy.detail.items.columns.unitPrice }),
    ).toBeTruthy();
    expect(view.container.textContent).toContain('780,00');
    expect(view.container.textContent).toContain('390.000,00');
    expect(view.queryByDisplayValue('780,00')).toBeNull();
  });

  it('groups thousands as the seller types, without them typing a single separator', async () => {
    const view = renderItems();
    const price = view.getByDisplayValue('780,00');

    fireEvent.change(price, { target: { value: '1' } });
    expect((price as HTMLInputElement).value).toBe('1');
    fireEvent.change(price, { target: { value: '1324' } });
    expect((price as HTMLInputElement).value).toBe('1.324');
    fireEvent.change(price, { target: { value: '132467' } });
    expect((price as HTMLInputElement).value).toBe('132.467');
    fireEvent.change(price, { target: { value: '132467,8' } });
    expect((price as HTMLInputElement).value).toBe('132.467,8');
  });

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
      expect(updateQuoteItem).toHaveBeenCalledWith(QUOTE_ID, ITEM_ID, BRANCH_ID, {
        unit_price_snapshot: '132467.89',
      }),
    );
  });

  /*
   * Removing a line cannot be undone by the screen — re-adding the product mints a new line and
   * re-prices it — so the trash asks first and only the confirmation writes.
   */
  it('asks before removing a line, and writes only once confirmed', async () => {
    vi.mocked(deleteQuoteItem).mockResolvedValue(undefined);
    const view = renderItems();

    fireEvent.click(view.getByRole('button', { name: copy.detail.items.delete }));

    expect(deleteQuoteItem).not.toHaveBeenCalled();
    const dialog = await vi.waitFor(() => view.getByRole('dialog'));
    expect(dialog.textContent).toContain(PRICED_ITEM.requested_description);

    fireEvent.click(view.getByRole('button', { name: copy.detail.items.deleteConfirm.confirm }));

    await vi.waitFor(() =>
      expect(deleteQuoteItem).toHaveBeenCalledWith(QUOTE_ID, ITEM_ID, BRANCH_ID),
    );
  });
});

describe('RfqItemsTable confidence', () => {
  // The badge renders the API's level, so a score the old cut-offs read as medium shows whatever
  // the backend's calibration decided it is.
  it.each([
    ['HIGH', '0.7500', 'MATCHED', copy.detail.confidence.high],
    ['MEDIUM', '0.9900', 'AMBIGUOUS', copy.detail.confidence.medium],
    ['LOW', '0.5400', 'NO_MATCH', copy.detail.confidence.low],
  ] as const)('shows %s from the API rather than from the score', (level, score, status, label) => {
    const view = renderItems(BRANCH_ID, 'DRAFT', [
      { ...PRICED_ITEM, confidence_level: level, confidence_score: score, match_status: status },
    ]);

    expect(view.getByText(label)).toBeTruthy();
  });

  it('marks a line the seller chose the product for as manual', () => {
    const view = renderItems(BRANCH_ID, 'DRAFT', [
      { ...PRICED_ITEM, confidence_level: null, confidence_score: null, match_status: 'MATCHED' },
    ]);

    expect(view.getByText(copy.detail.confidence.manual)).toBeTruthy();
  });

  it('says there is no data for a line matching never scored', () => {
    const view = renderItems(BRANCH_ID, 'DRAFT', [
      { ...PRICED_ITEM, confidence_level: null, confidence_score: null, match_status: 'NO_MATCH' },
    ]);

    expect(view.getByText(copy.detail.confidence.none)).toBeTruthy();
  });
});

describe('RfqItemsTable choosing a product for a flagged line', () => {
  it('lets an unmatched line pick from the candidates matching offered for it', async () => {
    const unmatched: QuoteItemResponse = {
      ...PRICED_ITEM,
      product_id: null,
      product_code: null,
      product_name: null,
      product_unit: null,
      unit_price_snapshot: null,
      min_price_snapshot: null,
      subtotal: null,
      confidence_score: '0.4200',
      confidence_level: 'LOW',
      match_status: 'NO_MATCH',
      alternatives: [
        {
          id: 'a0000000-0000-4000-8000-000000000001',
          product_id: 'p0000000-0000-4000-8000-000000000009',
          combo_id: null,
          type: 'PRODUCT',
          origin: 'AI',
          rank: 1,
          confidence_score: '0.4200',
          price_snapshot: null,
          approved_by_seller: false,
          chosen_by_client: false,
          code: 'LAD-HUE-18',
          canonical_name: 'Ladrillo hueco 18x18x33',
          unit: 'unidad',
        },
      ],
    };
    const view = renderItems(BRANCH_ID, 'DRAFT', [unmatched]);

    fireEvent.click(
      view.getByRole('button', { name: new RegExp(copy.detail.items.chooseProduct) }),
    );

    expect(await view.findByText('Ladrillo hueco 18x18x33')).toBeTruthy();
    expect(view.getByText(copy.detail.items.suggested)).toBeTruthy();
    // A line with no product has nothing to replace yet.
    expect(view.getByRole('heading', { name: copy.detail.items.chooseProduct })).toBeTruthy();
  });
});
