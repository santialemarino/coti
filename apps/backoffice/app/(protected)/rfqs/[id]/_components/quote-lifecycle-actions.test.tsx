import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { reactivateQuote, transitionQuote } from '@/lib/api/rfqs-client';
import messages from '@/translations/es.json';
import { QuoteLifecycleActions } from './quote-lifecycle-actions';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({
  reactivateQuote: vi.fn(),
  transitionQuote: vi.fn(),
}));

const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const copy = messages.rfqs.detail.lifecycle;

function renderActions(status: string, onChanged = vi.fn().mockResolvedValue(undefined)) {
  return {
    onChanged,
    view: render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <QuoteLifecycleActions
          quoteId={QUOTE_ID}
          branchId={BRANCH_ID}
          status={status}
          onChanged={onChanged}
        />
      </NextIntlClientProvider>,
    ),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(transitionQuote).mockResolvedValue({} as never);
  vi.mocked(reactivateQuote).mockResolvedValue({} as never);
});

describe('QuoteLifecycleActions', () => {
  it('lets the seller record an acceptance received outside Coti', async () => {
    const { view, onChanged } = renderActions('SENT');

    fireEvent.click(view.getByRole('button', { name: copy.accept.button }));
    fireEvent.click(view.getByRole('button', { name: copy.accept.confirm }));

    await waitFor(() =>
      expect(transitionQuote).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'ACCEPTED'),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it('keeps resend on the same version when that reactivation mode is chosen', async () => {
    const { view, onChanged } = renderActions('ACCEPTED');

    fireEvent.click(view.getByRole('button', { name: copy.reactivate.button }));
    fireEvent.click(view.getByRole('button', { name: new RegExp(copy.reactivate.resend.title) }));

    await waitFor(() =>
      expect(reactivateQuote).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'RESEND'),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it('opens a new editable version when edit and resend is chosen', async () => {
    const { view } = renderActions('REJECTED');

    fireEvent.click(view.getByRole('button', { name: copy.reactivate.button }));
    fireEvent.click(view.getByRole('button', { name: new RegExp(copy.reactivate.edit.title) }));

    await waitFor(() => expect(reactivateQuote).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'EDIT'));
  });
});
