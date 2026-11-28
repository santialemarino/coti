import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import messages from '@/translations/es.json';
import { ReplyAcceptanceCallout } from './reply-acceptance-callout';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ settleReplyAcceptance: vi.fn() }));

const { toast } = await import('sonner');
const { settleReplyAcceptance } = await import('@/lib/api/rfqs-client');

const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const copy = messages.rfqs.detail.callouts.replyAcceptance;

function renderCallout(
  props: { unconfirmed: boolean; undoable: boolean; readOnly?: boolean },
  onChanged = vi.fn().mockResolvedValue(undefined),
) {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ReplyAcceptanceCallout
        quoteId={QUOTE_ID}
        branchId={BRANCH_ID}
        reference="#07"
        onChanged={onChanged}
        {...props}
      />
    </NextIntlClientProvider>,
  );
  return { view, onChanged };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(settleReplyAcceptance).mockResolvedValue({} as never);
});

describe('ReplyAcceptanceCallout', () => {
  it('confirms an acceptance waiting for the seller in one click', async () => {
    const { view, onChanged } = renderCallout({ unconfirmed: true, undoable: false });

    expect(view.getByText(copy.unconfirmedTitle)).toBeTruthy();
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));

    await waitFor(() =>
      expect(settleReplyAcceptance).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'confirm'),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  // Undoing hands the quote back to the customer, so it asks first.
  it('undoes an acceptance only after the seller confirms the dialog', async () => {
    const { view, onChanged } = renderCallout({ unconfirmed: true, undoable: false });

    fireEvent.click(view.getByRole('button', { name: copy.undo }));
    expect(settleReplyAcceptance).not.toHaveBeenCalled();
    fireEvent.click(view.getByRole('button', { name: copy.undoConfirm }));

    await waitFor(() =>
      expect(settleReplyAcceptance).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'undo'),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it('keeps only the undo on a confirmed acceptance nothing has closed', () => {
    const { view } = renderCallout({ unconfirmed: false, undoable: true });

    expect(view.getByText(copy.confirmedTitle)).toBeTruthy();
    expect(view.getByRole('button', { name: copy.undo })).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.confirm })).toBeNull();
  });

  it('offers nothing on an archived quote', () => {
    const { view } = renderCallout({ unconfirmed: true, undoable: false, readOnly: true });

    expect(view.queryByRole('button')).toBeNull();
  });

  // The server decides whether it can still be undone; its refusal is worded, not generic.
  it('words the refusal when the acceptance can no longer be undone', async () => {
    vi.mocked(settleReplyAcceptance).mockRejectedValue(
      new ApiError('QUOTE_ACCEPTANCE_NOT_UNDOABLE', 409),
    );
    const { view, onChanged } = renderCallout({ unconfirmed: false, undoable: true });

    fireEvent.click(view.getByRole('button', { name: copy.undo }));
    fireEvent.click(view.getByRole('button', { name: copy.undoConfirm }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(messages.errors.QUOTE_ACCEPTANCE_NOT_UNDOABLE),
    );
    expect(onChanged).not.toHaveBeenCalled();
  });
});
