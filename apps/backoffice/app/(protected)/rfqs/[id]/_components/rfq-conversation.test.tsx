import { act, fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ConversationBatch, ConversationResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { RfqConversation } from './rfq-conversation';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ retryConversationBatch: vi.fn() }));

const { toast } = await import('sonner');
const { retryConversationBatch } = await import('@/lib/api/rfqs-client');

const copy = messages.rfqs.detail.conversation;
const BASE_TIME = '2026-10-15T14:00:00.000Z';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';

function batch(id: string, overrides: Partial<ConversationBatch>): ConversationBatch {
  return {
    id,
    status: 'PROCESSED',
    closed_at: BASE_TIME,
    failure_reason: null,
    retryable: false,
    messages: [{ author: 'CLIENT', body: `mensaje ${id}`, received_at: BASE_TIME }],
    decision: null,
    ...overrides,
  };
}

function renderConversation(conversation: ConversationResponse, onChanged = vi.fn()) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqConversation
        conversation={conversation}
        quoteId={QUOTE_ID}
        branchId={BRANCH_ID}
        onChanged={onChanged}
      />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RfqConversation', () => {
  it("shows each batch's messages, how the AI read them and what the seller did", () => {
    const view = renderConversation({
      batches: [
        batch('a', {
          decision: {
            interpretation: 'Pide 4 bolsas',
            outcome: 'APPLIED',
            no_match_reason: null,
            seller_decision: null,
            decided_at: null,
            created_at: BASE_TIME,
          },
        }),
        batch('b', {
          decision: {
            interpretation: '',
            outcome: 'NO_MATCH',
            no_match_reason: 'AI_DISABLED',
            seller_decision: 'MANUAL_OVERRIDE',
            decided_at: BASE_TIME,
            created_at: BASE_TIME,
          },
        }),
        batch('c', { status: 'CLOSED' }),
      ],
      pending: [{ author: 'CLIENT', body: 'y una carretilla', received_at: BASE_TIME }],
      seller_messages: [],
      can_discard_ai_changes: true,
    });

    expect(view.getByText('mensaje a')).toBeTruthy();
    expect(
      view.getByText(copy.decision.applied.replace('{interpretation}', 'Pide 4 bolsas')),
    ).toBeTruthy();
    expect(view.getByText(copy.sellerDecision.pending)).toBeTruthy();
    expect(view.getByText(copy.decision.noMatch.AI_DISABLED)).toBeTruthy();
    expect(view.getByText(copy.sellerDecision.MANUAL_OVERRIDE)).toBeTruthy();
    expect(view.getByText(copy.status.CLOSED)).toBeTruthy();
    expect(view.getByText('y una carretilla')).toBeTruthy();
    expect(view.getByText(copy.pending)).toBeTruthy();
  });

  it('says why a batch failed and offers a retry only when one can help', async () => {
    const onChanged = vi.fn().mockResolvedValue(undefined);
    vi.mocked(retryConversationBatch).mockResolvedValue(undefined);
    const view = renderConversation(
      {
        batches: [
          batch('closed', { status: 'FAILED', failure_reason: 'UNPROCESSABLE' }),
          batch('exhausted', {
            status: 'FAILED',
            failure_reason: 'ATTEMPTS_EXHAUSTED',
            retryable: true,
          }),
        ],
        pending: [],
        seller_messages: [],
        can_discard_ai_changes: false,
      },
      onChanged,
    );

    expect(view.getByText(copy.failure.UNPROCESSABLE)).toBeTruthy();
    expect(view.getByText(copy.failure.ATTEMPTS_EXHAUSTED)).toBeTruthy();
    const retries = view.getAllByRole('button', { name: copy.retry });
    expect(retries).toHaveLength(1);

    await act(async () => {
      fireEvent.click(retries[0]!);
    });

    expect(retryConversationBatch).toHaveBeenCalledWith(QUOTE_ID, 'exhausted', BRANCH_ID);
    expect(toast.success).toHaveBeenCalledWith(copy.retried);
    expect(onChanged).toHaveBeenCalled();
  });

  it('sets each send between the rounds it split, with the link read as a placeholder', () => {
    const at = (minutes: number) =>
      new Date(Date.parse(BASE_TIME) + minutes * 60_000).toISOString();
    const send = (minutes: number, text: string) => ({
      author: 'SELLER' as const,
      body: `${text} {{public_url}}`,
      received_at: at(minutes),
    });
    const view = renderConversation({
      batches: [
        batch('second', {
          messages: [{ author: 'CLIENT', body: 'segunda ronda', received_at: at(30) }],
        }),
        batch('first', {
          messages: [{ author: 'CLIENT', body: 'primera ronda', received_at: at(10) }],
        }),
      ],
      pending: [],
      seller_messages: [send(20, 'Te mando la v2:'), send(0, 'Te mando la v1:')],
      can_discard_ai_changes: false,
    });

    const order = ['Te mando la v1:', 'primera ronda', 'Te mando la v2:', 'segunda ronda'].map(
      (text) => view.container.textContent!.indexOf(text),
    );
    expect(order.every((position) => position >= 0)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
    expect(view.getAllByText(copy.seller)).toHaveLength(2);
    expect(view.getByText(`Te mando la v1: ${copy.quoteLink}`)).toBeTruthy();
    expect(view.container.textContent).not.toContain('{{public_url}}');
  });

  it.each(['PIPELINE_EXHAUSTED', 'PROMOTION_NOT_EARNED'] as const)(
    'words the %s no-match',
    (reason) => {
      const view = renderConversation({
        batches: [
          batch('a', {
            decision: {
              interpretation: 'Lo ve caro',
              outcome: 'NO_MATCH',
              no_match_reason: reason,
              seller_decision: null,
              decided_at: null,
              created_at: BASE_TIME,
            },
          }),
        ],
        pending: [],
        seller_messages: [],
        can_discard_ai_changes: false,
      });

      expect(view.getByText(copy.decision.noMatch[reason])).toBeTruthy();
    },
  );
});
