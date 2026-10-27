import { act, fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import type { ChangeRequestDiff, ConversationResponse, DiffLineItem } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { RfqChangeRequestCard } from './rfq-change-request-card';

vi.mock('@/lib/api/rfqs-client', () => ({ retryConversationBatch: vi.fn() }));

const copy = messages.rfqs.detail.changeRequest;
const BASE_TIME = '2026-10-15T14:00:00.000Z';

function side(product: string, quantity: string, price: string): DiffLineItem {
  return {
    description: product,
    product_name: product,
    quantity,
    unit: 'bolsas',
    unit_price: price,
  };
}

const DIFF: ChangeRequestDiff = {
  reason: null,
  client_messages: [{ body: 'Está muy caro', received_at: BASE_TIME }],
  handler: {
    client_input: 'Está muy caro',
    interpretation: 'Pide alternativas más baratas.',
    outcome: 'APPLIED',
    no_match_reason: null,
    created_at: BASE_TIME,
  },
  lines: [
    {
      change: 'MODIFIED',
      original: side('Cemento Loma Negra 50 kg', '20.00', '9500.00'),
      requested: side('Cemento Pampa 50 kg', '20.00', '8400.00'),
    },
    {
      change: 'UNCHANGED',
      original: side('Cal hidratada 25 kg', '5.00', '4200.00'),
      requested: side('Cal hidratada 25 kg', '5.00', '4200.00'),
    },
    { change: 'REMOVED', original: side('Arena gruesa', '3.00', '32000.00'), requested: null },
  ],
  discounts: [
    {
      change: 'MODIFIED',
      name: '5% en compras desde $500.000',
      original: { amount: '80700.00', suppressed: false, origin: 'AUTOMATIC' },
      requested: { amount: '71460.00', suppressed: true, origin: 'AUTOMATIC' },
    },
  ],
  original_total: '200000.00',
  requested_total: '180000.00',
};

const CONVERSATION: ConversationResponse = {
  batches: [
    {
      id: 'b1',
      status: 'PROCESSED',
      closed_at: BASE_TIME,
      failure_reason: null,
      retryable: false,
      messages: [{ author: 'CLIENT', body: 'mensaje del historial', received_at: BASE_TIME }],
      decision: null,
    },
  ],
  pending: [],
  seller_messages: [],
  can_discard_ai_changes: true,
};

function renderCard(diff = DIFF, onDiscard = vi.fn()) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqChangeRequestCard
        diff={diff}
        variant="customer-request"
        conversation={CONVERSATION}
        quoteId="q1"
        branchId="br1"
        canDiscard
        onDiscard={onDiscard}
        onChanged={vi.fn()}
      />
    </NextIntlClientProvider>,
  );
}

describe('RfqChangeRequestCard', () => {
  it('quotes the customer and states the reading without naming the AI', () => {
    const view = renderCard();

    expect(view.container.querySelector('blockquote')?.textContent).toContain('Está muy caro');
    expect(view.getByText(copy.interpretation, { exact: false })).toBeTruthy();
    expect(view.getByText('Pide alternativas más baratas.')).toBeTruthy();
    expect(view.getByText(copy.outcome.applied)).toBeTruthy();
  });

  // Only what moved is listed; the rest is counted, so the card stays short.
  it('lists only the changes, each line beside the one it replaced', () => {
    const view = renderCard();

    const text = view.container.textContent ?? '';
    expect(text).toContain('Cemento Loma Negra 50 kg');
    expect(text).toContain('Cemento Pampa 50 kg');
    expect(view.queryByText('Cal hidratada 25 kg')).toBeNull();
    expect(view.getByText('1 ítem sin cambios')).toBeTruthy();
    expect(view.getAllByText(copy.change.MODIFIED).length).toBe(2);
    expect(view.getAllByText(copy.change.REMOVED).length).toBe(1);
    expect(text).toContain(copy.suppressed);
  });

  it('shows the total before and after with the change in percent', () => {
    const view = renderCard();

    const text = (view.container.textContent ?? '').replace(/\s/g, ' ');
    expect(text).toMatch(/200\.000,00.*180\.000,00/);
    expect(text).toMatch(/-10\s?%/);
  });

  it('keeps the conversation folded until asked, with its rounds counted', async () => {
    const view = renderCard();

    expect(view.queryByText('mensaje del historial')).toBeNull();
    const trigger = view.getByRole('button', { name: new RegExp(copy.conversation) });
    expect(trigger.textContent).toContain('1 ronda');
    await act(async () => {
      fireEvent.click(trigger);
    });
    expect(view.getByText('mensaje del historial')).toBeTruthy();
  });

  /*
   * A promotion the customer asked for by name was reached by raising a quantity: the card says
   * which line moved and to what, that the total went up because of it, which strategy ran, and
   * marks the discount as the AI's — every enum through the catalog, never raw.
   */
  it('explains a quantity raised to reach a promotion and the higher total', () => {
    const recipe = messages.rfqs.detail.handlerRecipe;
    const view = renderCard({
      ...DIFF,
      handler: {
        ...DIFF.handler!,
        strategies_tried: ['ADAPT_TO_REACH_DISCOUNT'],
        adaptation: {
          lines: [
            {
              item_id: 'i1',
              description: 'Cemento Loma Negra 50 kg',
              from_quantity: '15.00',
              to_quantity: '20.00',
              promotion_id: 'p1',
              promotion_name: 'Cemento x20',
            },
          ],
          total_before: '142500.00',
          total_after: '171000.00',
          total_raised: true,
        },
      },
      discounts: [
        {
          change: 'ADDED',
          name: 'Cemento x20',
          original: null,
          requested: { amount: '19000.00', suppressed: false, origin: 'AI_ADAPTATION' },
        },
      ],
    });

    const text = (view.container.textContent ?? '').replace(/\s/g, ' ');
    expect(text).toContain(
      'Se subió Cemento Loma Negra 50 kg de 15 a 20 para alcanzar «Cemento x20».',
    );
    expect(text).toMatch(/El total sube de \$ 142\.500,00 a \$ 171\.000,00/);
    expect(text).toContain(recipe.strategy.ADAPT_TO_REACH_DISCOUNT);
    expect(text).not.toContain('ADAPT_TO_REACH_DISCOUNT');
    expect(view.getByText(copy.adaptedDiscount)).toBeTruthy();
  });

  it('hands the discard to the caller', () => {
    const onDiscard = vi.fn();
    const view = renderCard(DIFF, onDiscard);

    fireEvent.click(view.getByRole('button', { name: copy.discard }));

    expect(onDiscard).toHaveBeenCalled();
  });
});
