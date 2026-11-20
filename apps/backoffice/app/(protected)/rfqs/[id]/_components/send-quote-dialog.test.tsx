import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { getClientDirectory, getQuoteClientAssociation } from '@/lib/api/clients-client';
import type {
  QuoteDeliveryMode,
  QuoteDeliveryPreset,
  QuoteDeliveryResponse,
  QuoteSendResponse,
  RfqDetailResponse,
} from '@/lib/api/rfqs';
import { draftQuoteFollowup, fetchDeliveryPreset, sendQuote } from '@/lib/api/rfqs-client';
import messages from '@/translations/es.json';
import { initialSelection, SendQuoteDialog } from './send-quote-dialog';

vi.mock('@/lib/api/clients-client', () => ({
  getClientDirectory: vi.fn(),
  getQuoteClientAssociation: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({
  sendQuote: vi.fn(),
  draftQuoteFollowup: vi.fn(),
  fetchDeliveryPreset: vi.fn(),
}));
vi.mock('@/app/(protected)/rfqs/_components/rfq-list-context', () => ({
  useRfqList: () => ({ isAdmin: true }),
}));

const { toast } = await import('sonner');

const copy = messages.rfqs.detail.send;

const BASE_TIME = '2026-09-07T20:45:00.000Z';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const RFQ_ID = '10000000-0000-4000-8000-000000000002';
const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const VERSION_ID = '30000000-0000-4000-8000-000000000002';
const SELLER_ID = '50000000-0000-4000-8000-000000000001';
const PHONE = '+5491155550101';
const WHATSAPP_URL = 'https://coti.app/quotes/wa-token-a';

const writeText = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));

function makeDetail(inboundWhatsAppPhone: string | null = null): RfqDetailResponse {
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
      review_count: 0,
      total: '390000.00',
      status: 'QUOTED',
      needs_followup: false,
      followup_flagged_at: null,
      paid_at: null,
      paid_by: null,
      invoice_waived_at: null,
      invoice_waived_by: null,
      invoiced: false,
      invoice_pending: false,
      sale_complete: false,
      archived_at: null,
    },
    inbound_whatsapp_phone: inboundWhatsAppPhone,
    inbound_email: null,
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
      paid_at: null,
      paid_by_user_id: null,
      invoice_waived_at: null,
      invoice_waived_by_user_id: null,
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

function renderDialog(
  inboundWhatsAppPhone: string | null = null,
  detail = makeDetail(inboundWhatsAppPhone),
  mode: QuoteDeliveryMode = 'FIRST',
) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <SendQuoteDialog
        mode={mode}
        detail={detail}
        branchId={BRANCH_ID}
        onSent={vi.fn().mockResolvedValue(undefined)}
      />
    </NextIntlClientProvider>,
  );
}

function openDialog(view: RenderResult, mode: Exclude<QuoteDeliveryMode, 'FOLLOWUP'> = 'FIRST') {
  fireEvent.click(view.getByRole('button', { name: copy.modes[mode].trigger }));
}

function preset(overrides: Partial<QuoteDeliveryPreset> = {}): QuoteDeliveryPreset {
  return { source: 'NONE', phones: [], emails: [], usedPhones: [], usedEmails: [], ...overrides };
}

const client = {
  id: 'client-1',
  taxId: null,
  name: 'Constructora',
  phone: PHONE,
  email: 'compras@example.test',
  originChannel: null,
  notes: null,
  createdAt: BASE_TIME,
  updatedAt: BASE_TIME,
};
function association(
  overrides: Partial<
    Omit<typeof client, 'phone' | 'email'> & { phone: string | null; email: string | null }
  > = {},
) {
  return {
    currentClient: { client: { ...client, ...overrides }, tags: [] },
    suggestions: [],
    availableTags: [],
    contactHints: { name: null, phone: null, email: null },
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getClientDirectory).mockResolvedValue([]);
  vi.mocked(getQuoteClientAssociation).mockResolvedValue(association());
  vi.mocked(sendQuote).mockResolvedValue(sentResult([delivery({ channel: 'WHATSAPP' })]));
  vi.mocked(fetchDeliveryPreset).mockResolvedValue(preset());
  vi.mocked(draftQuoteFollowup).mockResolvedValue({
    message: 'Mensaje de seguimiento',
    templateConfigured: false,
  });
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
});
afterEach(() => vi.unstubAllGlobals());
async function ready(view: RenderResult) {
  await waitFor(() =>
    expect(
      view
        .getByRole('dialog')
        .querySelector('[data-slot=dialog-footer] button:last-child')
        ?.hasAttribute('disabled'),
    ).toBe(false),
  );
}
async function submit(view: RenderResult) {
  await ready(view);
  fireEvent.click(
    view.getByRole('button', {
      name: view.queryByRole('button', { name: messages.rfqs.detail.callouts.followup.confirmSend })
        ? messages.rfqs.detail.callouts.followup.confirmSend
        : copy.send,
    }),
  );
  await waitFor(() => expect(sendQuote).toHaveBeenCalled());
}

describe('seller delivery destinations', () => {
  it('prefills both saved contacts and selects WhatsApp alone', async () => {
    const view = renderDialog();
    openDialog(view);
    await ready(view);
    expect(
      view.getByRole('checkbox', { name: copy.sendWhatsApp }).getAttribute('aria-checked'),
    ).toBe('true');
    expect(view.getByRole('checkbox', { name: copy.alsoEmail }).getAttribute('aria-checked')).toBe(
      'false',
    );
    expect((view.getByLabelText(copy.emailLabel) as HTMLInputElement).value).toBe(client.email);
    await submit(view);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({ recipientPhones: [PHONE], recipientEmails: [] }),
      expect.any(String),
    );
  });
  it('chooses email when the client has no phone', async () => {
    vi.mocked(getQuoteClientAssociation).mockResolvedValue(association({ phone: null }));
    const view = renderDialog();
    openDialog(view);
    await ready(view);
    expect(
      view.getByRole('checkbox', { name: copy.sendWhatsApp }).getAttribute('aria-checked'),
    ).toBe('false');
    expect(view.getByRole('checkbox', { name: copy.alsoEmail }).getAttribute('aria-checked')).toBe(
      'true',
    );
  });
  it('leaves every channel unchecked when no contact exists', async () => {
    vi.mocked(getQuoteClientAssociation).mockResolvedValue(
      association({ phone: null, email: null }),
    );
    const view = renderDialog();
    openDialog(view);
    await view.findByText(client.name);
    expect(
      view.getByRole('checkbox', { name: copy.sendWhatsApp }).getAttribute('aria-checked'),
    ).toBe('false');
    expect(view.getByRole('checkbox', { name: copy.alsoEmail }).getAttribute('aria-checked')).toBe(
      'false',
    );
    expect(view.getByRole('button', { name: copy.send }).hasAttribute('disabled')).toBe(true);
  });
  it('disables every medium the send cycle reached and points to «Reenviar»', async () => {
    vi.mocked(fetchDeliveryPreset).mockResolvedValue(
      preset({ usedPhones: [PHONE], usedEmails: [client.email!] }),
    );
    const view = renderDialog(null, makeDetail(), 'OTHER_MEDIUM');
    openDialog(view, 'OTHER_MEDIUM');
    await view.findByText(client.name);
    await view.findByText(copy.allUsed);
    for (const channel of [copy.sendWhatsApp, copy.alsoEmail]) {
      const box = view.getByRole('checkbox', { name: new RegExp(channel) });
      expect(box.hasAttribute('disabled')).toBe(true);
      expect(box.getAttribute('aria-checked')).toBe('false');
    }
    expect(view.getByText(copy.usedOn.replace('{destinations}', PHONE))).toBeTruthy();
    expect(view.getByRole('button', { name: copy.send }).hasAttribute('disabled')).toBe(true);
  });
  it('offers the medium the cycle did not reach and sends it as another medium', async () => {
    vi.mocked(fetchDeliveryPreset).mockResolvedValue(preset({ usedPhones: [PHONE] }));
    const view = renderDialog(null, makeDetail(), 'OTHER_MEDIUM');
    openDialog(view, 'OTHER_MEDIUM');
    await ready(view);
    expect(view.getByRole('checkbox', { name: copy.alsoEmail }).getAttribute('aria-checked')).toBe(
      'true',
    );
    await submit(view);
    expect(fetchDeliveryPreset).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, 'OTHER_MEDIUM', null);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({
        mode: 'OTHER_MEDIUM',
        recipientPhones: [],
        recipientEmails: [client.email],
        clientAssociation: undefined,
      }),
      expect.any(String),
    );
  });
  it('preselects exactly the destinations of the previous version and says so', async () => {
    const earlier = '+5491155550999';
    vi.mocked(fetchDeliveryPreset).mockResolvedValue(
      preset({ source: 'PREVIOUS_VERSION', phones: [earlier], emails: [client.email!] }),
    );
    const view = renderDialog();
    openDialog(view);
    await ready(view);
    expect(view.getByText(copy.presetSource.PREVIOUS_VERSION)).toBeTruthy();
    for (const channel of [copy.sendWhatsApp, copy.alsoEmail]) {
      expect(view.getByRole('checkbox', { name: channel }).getAttribute('aria-checked')).toBe(
        'true',
      );
    }
    await submit(view);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({
        mode: 'FIRST',
        recipientPhones: [earlier],
        recipientEmails: [client.email],
      }),
      expect.any(String),
    );
  });
  it('asks the preset of the client chosen in the dialog', async () => {
    const view = renderDialog();
    openDialog(view);
    await ready(view);
    await waitFor(() =>
      expect(fetchDeliveryPreset).toHaveBeenLastCalledWith(QUOTE_ID, BRANCH_ID, 'FIRST', client.id),
    );
  });
  it('resends to what the cycle reached, all media open', async () => {
    vi.mocked(fetchDeliveryPreset).mockResolvedValue(
      preset({ source: 'VERSION', phones: [PHONE], usedPhones: [PHONE] }),
    );
    const view = renderDialog(null, makeDetail(), 'RESEND');
    openDialog(view, 'RESEND');
    await ready(view);
    expect(view.getByRole('checkbox', { name: copy.sendWhatsApp }).hasAttribute('disabled')).toBe(
      false,
    );
    await submit(view);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({ mode: 'RESEND', recipientPhones: [PHONE] }),
      expect.any(String),
    );
  });
  it('supports selecting several destinations of one channel', async () => {
    vi.mocked(getQuoteClientAssociation).mockResolvedValue({
      ...association(),
      currentClient: {
        client: {
          ...client,
          contacts: [
            { kind: 'EMAIL', value: client.email!, primary: true },
            { kind: 'EMAIL', value: 'otro@example.test', primary: false },
          ],
        },
        tags: [],
      },
    });
    const view = renderDialog();
    openDialog(view);
    await ready(view);
    fireEvent.click(view.getByRole('checkbox', { name: copy.alsoEmail }));
    fireEvent.click(view.getByRole('combobox', { name: copy.emailLabel }));
    fireEvent.click(view.getByRole('option', { name: 'otro@example.test' }));
    fireEvent.keyDown(view.getByRole('combobox', { name: copy.emailLabel }), { key: 'Escape' });
    await submit(view);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({ recipientEmails: [client.email, 'otro@example.test'] }),
      expect.any(String),
    );
  });
  it('keeps an idempotency key when the same request is retried after a network error', async () => {
    vi.mocked(sendQuote).mockRejectedValueOnce(new Error('offline'));
    const view = renderDialog();
    openDialog(view);
    await submit(view);
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    const key = vi.mocked(sendQuote).mock.calls[0]![3];
    await ready(view);
    fireEvent.click(view.getByRole('button', { name: copy.send }));
    await waitFor(() => expect(sendQuote).toHaveBeenCalledTimes(2));
    expect(vi.mocked(sendQuote).mock.calls[1]![3]).toBe(key);
  });
  it('reports partial delivery without hiding the successful channel', async () => {
    vi.mocked(sendQuote).mockResolvedValue(
      sentResult([
        delivery({ channel: 'WHATSAPP' }),
        delivery({ channel: 'EMAIL', tracking_status: 'FAILED' }),
      ]),
    );
    const view = renderDialog();
    openDialog(view);
    await submit(view);
    await waitFor(() => expect(toast.warning).toHaveBeenCalled());
  });
  it('does not expose the customer-only URL', async () => {
    const view = renderDialog();
    openDialog(view);
    await submit(view);
    expect(view.queryByText(WHATSAPP_URL)).toBeNull();
  });
});

describe('initialSelection', () => {
  const both = { phones: [PHONE], emails: [client.email!] };
  const cases: [
    string,
    Parameters<typeof initialSelection>,
    ReturnType<typeof initialSelection>,
  ][] = [
    [
      'WhatsApp alone when both are known, email still prefilled',
      [both, preset(), 'FIRST'],
      { ...both, whatsapp: true, email: false },
    ],
    [
      'email when only email is known',
      [{ phones: [], emails: both.emails }, preset(), 'FIRST'],
      { phones: [], emails: both.emails, whatsapp: false, email: true },
    ],
    [
      'nothing checked without data',
      [{ phones: [], emails: [] }, preset(), 'FIRST'],
      { phones: [], emails: [], whatsapp: false, email: false },
    ],
    [
      'a preset selects exactly what it names',
      [both, preset({ source: 'CLIENT_LAST', emails: ['otro@example.test'] }), 'FIRST'],
      { phones: [], emails: ['otro@example.test'], whatsapp: false, email: true },
    ],
    [
      'another medium skips the used one',
      [both, preset({ usedPhones: [PHONE] }), 'OTHER_MEDIUM'],
      { ...both, whatsapp: false, email: true },
    ],
  ];
  it.each(cases)('%s', (_, args, want) => {
    expect(initialSelection(...args)).toEqual(want);
  });
});

describe('seller-confirmed follow-up', () => {
  it('loads a deterministic template and sends only after confirmation, with the original recipients selected', async () => {
    vi.mocked(fetchDeliveryPreset).mockResolvedValue(
      preset({ source: 'VERSION', emails: [client.email!], usedEmails: [client.email!] }),
    );
    const view = renderDialog(null, makeDetail(), 'FOLLOWUP');
    fireEvent.click(
      view.getByRole('button', { name: messages.rfqs.detail.callouts.followup.send }),
    );
    await ready(view);
    expect(sendQuote).not.toHaveBeenCalled();
    expect(view.getByRole('checkbox', { name: copy.alsoEmail }).getAttribute('aria-checked')).toBe(
      'true',
    );
    expect(
      view.getByRole('checkbox', { name: copy.sendWhatsApp }).getAttribute('aria-checked'),
    ).toBe('false');
    fireEvent.click(
      view.getByRole('button', { name: messages.rfqs.detail.callouts.followup.edit }),
    );
    fireEvent.change(
      view.getByRole('textbox', { name: messages.rfqs.detail.callouts.followup.draft.label }),
      { target: { value: 'Mensaje personalizado' } },
    );
    await submit(view);
    expect(sendQuote).toHaveBeenCalledWith(
      QUOTE_ID,
      BRANCH_ID,
      expect.objectContaining({
        mode: 'FOLLOWUP',
        recipientEmails: [client.email],
        followupMessage: 'Mensaje personalizado',
      }),
      expect.any(String),
    );
  });
  it('offers configuration only while the account has never customized its template', async () => {
    vi.mocked(draftQuoteFollowup).mockResolvedValue({
      message: 'Personalizado',
      templateConfigured: true,
    });
    const view = renderDialog(null, makeDetail(), 'FOLLOWUP');
    fireEvent.click(
      view.getByRole('button', { name: messages.rfqs.detail.callouts.followup.send }),
    );
    await ready(view);
    expect(
      view.queryByRole('link', { name: messages.rfqs.detail.callouts.followup.configure }),
    ).toBeNull();
  });
});
