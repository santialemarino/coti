import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import type { QuoteDeliveryResponse, QuoteSendResponse, RfqDetailResponse } from '@/lib/api/rfqs';
import { sendQuote } from '@/lib/api/rfqs-client';
import messages from '@/translations/es.json';
import { SendQuoteDialog } from './send-quote-dialog';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ sendQuote: vi.fn() }));

const { toast } = await import('sonner');

const copy = messages.rfqs.detail.send;
const channels = messages.rfqs.channels;

const BASE_TIME = '2026-09-07T20:45:00.000Z';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const RFQ_ID = '10000000-0000-4000-8000-000000000002';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const VERSION_ID = '30000000-0000-4000-8000-000000000002';
const SELLER_ID = '50000000-0000-4000-8000-000000000001';
const PHONE = '+5491155550101';
const WHATSAPP_URL = 'https://coti.app/quotes/wa-token-a';
const EMAIL_URL = 'https://coti.app/quotes/email-token-b';

const writeText = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));

function makeDetail(): RfqDetailResponse {
  return {
    rfq: {
      id: RFQ_ID,
      quote_id: QUOTE_ID,
      quote_number: 7,
      client: 'Constructora',
      created_at: BASE_TIME,
      channel: 'manual_entry',
      seller_id: SELLER_ID,
      seller: 'Admin Dev',
      branch: 'Villa Bosch',
      branch_id: BRANCH_ID,
      item_count: 1,
      total: '390000.00',
      status: 'QUOTED',
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
      current_status: 'QUOTED',
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
      total: '390000.00',
      is_immutable: false,
      comment: null,
      created_at: BASE_TIME,
    },
    items: [],
    alternatives: {},
    rfq_status_history: [],
    quote_status_history: [],
    deliveries: [],
    discounts: [],
  };
}

function delivery(overrides: Partial<QuoteDeliveryResponse>): QuoteDeliveryResponse {
  return {
    id: 'd-1',
    channel: 'whatsapp',
    destination: PHONE,
    tracking_status: 'SENT',
    public_url: WHATSAPP_URL,
    sent_at: BASE_TIME,
    ...overrides,
  };
}

function sentResult(deliveries: QuoteDeliveryResponse[]): QuoteSendResponse {
  return {
    quote_id: QUOTE_ID,
    version_id: VERSION_ID,
    current_status: 'SENT',
    expires_at: '2026-09-22T12:00:00Z',
    deliveries,
  };
}

function renderDialog() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <SendQuoteDialog
        detail={makeDetail()}
        branchId={BRANCH_ID}
        onSent={vi.fn().mockResolvedValue(undefined)}
      />
    </NextIntlClientProvider>,
  );
}

function openDialog(view: RenderResult) {
  fireEvent.click(view.getByRole('button', { name: copy.button }));
}

// The required mark renders inside the label, so its accessible name is the text plus the asterisk.
function phoneField(view: RenderResult) {
  return view.getByLabelText(new RegExp(copy.phoneLabel));
}

async function sendOn(view: RenderResult, phone = PHONE) {
  fireEvent.change(phoneField(view), { target: { value: phone } });
  fireEvent.click(view.getByRole('button', { name: /^Enviar$/ }));
  await waitFor(() => expect(sendQuote).toHaveBeenCalled());
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
});

afterEach(() => {
  delete (navigator as { clipboard?: unknown }).clipboard;
});

describe('SendQuoteDialog', () => {
  /*
   * The send is done: what the seller still needs is the link. It rides the same two channels it
   * was sent on, so the dialog holds it until it is closed rather than disappearing with the toast.
   */
  it('keeps the dialog open on success and surfaces each delivered link', async () => {
    vi.mocked(sendQuote).mockResolvedValue(sentResult([delivery({})]));
    const view = renderDialog();
    openDialog(view);
    await sendOn(view);

    expect(view.getByText(copy.successTitle)).toBeTruthy();
    expect(view.getByText(WHATSAPP_URL)).toBeTruthy();
    expect(view.getByRole('link', { name: /Abrir/ }).getAttribute('href')).toBe(WHATSAPP_URL);
  });

  it('copies a delivered link, then says so on the button', async () => {
    vi.mocked(sendQuote).mockResolvedValue(sentResult([delivery({})]));
    const view = renderDialog();
    openDialog(view);
    await sendOn(view);

    fireEvent.click(view.getByRole('button', { name: copy.copyLink }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(WHATSAPP_URL));
    expect(view.getByRole('button', { name: copy.copiedLink })).toBeTruthy();
  });

  /*
   * A channel that failed never delivered its link, so naming it here would hand the seller a URL
   * the customer never received — the toast already said which channel to retry by hand.
   */
  it('shows only the links of channels that delivered', async () => {
    vi.mocked(sendQuote).mockResolvedValue(
      sentResult([
        delivery({ id: 'd-wa', tracking_status: 'FAILED' }),
        delivery({
          id: 'd-em',
          channel: 'email',
          destination: 'obra@test.local',
          tracking_status: 'SENT',
          public_url: EMAIL_URL,
        }),
      ]),
    );
    const view = renderDialog();
    openDialog(view);
    await sendOn(view);

    expect(toast.warning).toHaveBeenCalledWith(expect.stringContaining(channels.whatsapp));
    expect(view.getByText(EMAIL_URL)).toBeTruthy();
    expect(view.queryByText(WHATSAPP_URL)).toBeNull();
  });

  /*
   * An all-failed send never reaches this dialog as a result: the API answers with an error the
   * client throws on, so the form must stay where it is rather than flip to a success screen.
   */
  it('stays on the form when every channel refused the send', async () => {
    vi.mocked(sendQuote).mockRejectedValue(new ApiError('UNREACHABLE', 0));
    const view = renderDialog();
    openDialog(view);
    await sendOn(view);

    expect(toast.error).toHaveBeenCalledWith(copy.errors.UNREACHABLE);
    expect(view.queryByText(copy.successTitle)).toBeNull();
    expect(phoneField(view)).toBeTruthy();
  });

  it('returns to the form from the success view to send to another destination', async () => {
    vi.mocked(sendQuote).mockResolvedValue(sentResult([delivery({})]));
    const view = renderDialog();
    openDialog(view);
    await sendOn(view);

    fireEvent.click(view.getByRole('button', { name: copy.resend }));

    expect(view.queryByText(copy.successTitle)).toBeNull();
    expect(phoneField(view)).toBeTruthy();
  });

  it('never sends while the phone is malformed', async () => {
    const view = renderDialog();
    openDialog(view);

    fireEvent.change(phoneField(view), { target: { value: '1155550101' } });
    fireEvent.click(view.getByRole('button', { name: /^Enviar$/ }));

    expect(sendQuote).not.toHaveBeenCalled();
  });
});
