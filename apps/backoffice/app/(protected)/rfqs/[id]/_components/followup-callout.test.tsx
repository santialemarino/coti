import { act, fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import messages from '@/translations/es.json';
import { FollowupCallout } from './followup-callout';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ draftQuoteFollowup: vi.fn() }));

const { draftQuoteFollowup } = await import('@/lib/api/rfqs-client');
const copy = messages.rfqs.detail.callouts.followup;

function renderCallout(origin: 'customer' | 'seller', quoteId: string | null = 'q-1') {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <FollowupCallout
        quoteId={quoteId}
        branchId="b-1"
        origin={origin}
        quietSince="2026-10-01T15:00:00Z"
      />
    </NextIntlClientProvider>,
  );
}

describe('FollowupCallout', () => {
  beforeEach(() => vi.clearAllMocks());

  it('drafts a message the seller can edit and copy for a client who went quiet', async () => {
    vi.mocked(draftQuoteFollowup).mockResolvedValue({ message: 'Hola Juan, ¿pudiste verla?' });
    renderCallout('customer');

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: copy.draft.action }));
    });

    expect(draftQuoteFollowup).toHaveBeenCalledWith('q-1', 'b-1');
    const field = screen.getByLabelText(copy.draft.label) as HTMLTextAreaElement;
    expect(field.value).toBe('Hola Juan, ¿pudiste verla?');
    fireEvent.change(field, { target: { value: 'Hola Juan, te escribo por la cotización' } });
    expect(field.value).toBe('Hola Juan, te escribo por la cotización');
    expect(screen.getByRole('button', { name: copy.draft.copy })).toBeTruthy();
    expect(await screen.findByRole('button', { name: copy.draft.again })).toBeTruthy();
  });

  // Before the quote is sent there is no client to write to: the seller sends the quote instead.
  it('offers no draft while the quote waits on the seller', () => {
    renderCallout('seller');

    expect(screen.getByText(copy.title)).toBeTruthy();
    expect(screen.queryByRole('button', { name: copy.draft.action })).toBeNull();
  });

  it('words a draft the model could not write', async () => {
    vi.mocked(draftQuoteFollowup).mockRejectedValue(new ApiError('AI_UNAVAILABLE', 503));
    renderCallout('customer');

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: copy.draft.action }));
    });

    expect(toast.error).toHaveBeenCalledWith(copy.draft.errors.AI_UNAVAILABLE);
    expect(screen.queryByLabelText(copy.draft.label)).toBeNull();
  });
});
