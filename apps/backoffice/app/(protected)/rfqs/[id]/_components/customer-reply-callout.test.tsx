import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CustomerReply } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { CustomerReplyCallout } from './customer-reply-callout';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ readCustomerReply: vi.fn(), transitionQuote: vi.fn() }));

const { readCustomerReply, transitionQuote } = await import('@/lib/api/rfqs-client');

const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const copy = messages.rfqs.detail.callouts.customerReply;

function renderCallout(
  reply: Pick<CustomerReply, 'reason' | 'replyAcceptance'>,
  onChanged = vi.fn().mockResolvedValue(undefined),
) {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CustomerReplyCallout
        quoteId={QUOTE_ID}
        branchId={BRANCH_ID}
        reference="#07"
        reply={reply}
        onChanged={onChanged}
      />
    </NextIntlClientProvider>,
  );
  return { view, onChanged };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(readCustomerReply).mockResolvedValue(undefined);
  vi.mocked(transitionQuote).mockResolvedValue({} as never);
});

describe('CustomerReplyCallout', () => {
  it('offers to mark an apparent acceptance accepted', async () => {
    const { view, onChanged } = renderCallout({
      reason: 'CUSTOMER_ACCEPTS',
      replyAcceptance: 'SIGNAL',
    });

    expect(view.getByText(copy.ACCEPTS.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.reject })).toBeNull();
    fireEvent.click(view.getByRole('button', { name: copy.accept }));

    await waitFor(() =>
      expect(transitionQuote).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'ACCEPTED'),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  // The AI never rejects; the seller does, in one click with no dialog in between.
  it('rejects an apparent rejection in one click', async () => {
    const { view } = renderCallout({ reason: 'CUSTOMER_REJECTS', replyAcceptance: null });

    expect(view.getByText(copy.REJECTS.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.accept })).toBeNull();
    fireEvent.click(view.getByRole('button', { name: copy.reject }));

    await waitFor(() =>
      expect(transitionQuote).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'REJECTED'),
    );
    expect(view.queryByRole('dialog')).toBeNull();
  });

  it.each([
    ['CUSTOMER_QUESTION', copy.QUESTION.title],
    ['NO_REQUEST', copy.WROTE.title],
  ] as const)('offers only «Marcar como leído» for %s', async (reason, title) => {
    const { view, onChanged } = renderCallout({ reason, replyAcceptance: null });

    expect(view.getByText(title)).toBeTruthy();
    expect(view.getAllByRole('button')).toHaveLength(1);
    fireEvent.click(view.getByRole('button', { name: copy.read }));

    await waitFor(() => expect(readCustomerReply).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID));
    expect(onChanged).toHaveBeenCalled();
    expect(transitionQuote).not.toHaveBeenCalled();
  });
});
