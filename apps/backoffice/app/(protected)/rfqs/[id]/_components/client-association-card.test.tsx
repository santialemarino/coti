import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ClientMatch, QuoteClientAssociation } from '@/lib/api/client-profiles';
import {
  associateQuoteClient,
  createClientTag,
  getClientDirectory,
  getQuoteClientAssociation,
} from '@/lib/api/clients-client';
import messages from '@/translations/es.json';
import { ClientAssociationCard } from './client-association-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({
  associateQuoteClient: vi.fn(),
  createClientTag: vi.fn(),
  getClientDirectory: vi.fn(),
  getQuoteClientAssociation: vi.fn(),
}));

const QUOTE_ID = '20000000-0000-4000-8000-000000000022';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const CLIENT_ID = 'c0000000-0000-4000-8000-000000000022';
const TAG_ID = 'a0000000-0000-4000-8000-000000000022';
const NOW = '2026-09-26T12:00:00Z';

const association: QuoteClientAssociation = {
  currentClient: null,
  suggestions: [
    {
      client: {
        id: CLIENT_ID,
        name: 'Constructora Horizonte',
        phone: '+5491155550101',
        email: 'compras@horizonte.test',
        originChannel: 'WHATSAPP',
        notes: null,
        createdAt: NOW,
        updatedAt: NOW,
      },
      tags: [{ id: TAG_ID, name: 'Recurrente', createdAt: NOW }],
    },
  ],
  contactHints: { phone: '+5491155550101', email: 'compras@horizonte.test' },
  availableTags: [{ id: TAG_ID, name: 'Recurrente', createdAt: NOW }],
};

function renderCard(onClientChange?: (clientId: string | null) => void) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ClientAssociationCard
        quoteId={QUOTE_ID}
        branchId={BRANCH_ID}
        onClientChange={onClientChange}
      />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getQuoteClientAssociation).mockResolvedValue(association);
  vi.mocked(getClientDirectory).mockResolvedValue([]);
  vi.mocked(associateQuoteClient).mockResolvedValue(association.suggestions[0]!);
});

describe('ClientAssociationCard', () => {
  it('keeps a contact match as a suggestion until the seller confirms it', async () => {
    const view = renderCard();

    expect(await view.findByText(messages.clients.association.pending.title)).toBeTruthy();
    expect(associateQuoteClient).not.toHaveBeenCalled();

    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));
    expect(await view.findByText('Constructora Horizonte')).toBeTruthy();
    expect(associateQuoteClient).not.toHaveBeenCalled();

    fireEvent.click(view.getByRole('button', { name: messages.clients.association.dialog.save }));

    await waitFor(() =>
      expect(associateQuoteClient).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, {
        client_id: CLIENT_ID,
        tag_ids: [TAG_ID],
      }),
    );
    expect(await view.findByRole('link', { name: 'Constructora Horizonte' })).toBeTruthy();
  });

  // The invoice card reads the sale's client from here, so it hears of a new one at once.
  it('reports the sale’s client once loaded and again after an association', async () => {
    const onClientChange = vi.fn();
    const view = renderCard(onClientChange);

    await waitFor(() => expect(onClientChange).toHaveBeenCalledWith(null));
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));
    fireEvent.click(
      await view.findByRole('button', { name: messages.clients.association.dialog.save }),
    );

    await waitFor(() => expect(onClientChange).toHaveBeenLastCalledWith(CLIENT_ID));
  });

  it('does not copy a suggested profile tags when the seller chooses a new client', async () => {
    const view = renderCard();
    await view.findByText(messages.clients.association.pending.title);
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));

    const tag = await view.findByRole('checkbox', { name: 'Recurrente' });
    expect(tag.getAttribute('data-state')).toBe('checked');

    fireEvent.click(view.getByRole('radio', { name: messages.clients.association.dialog.new }));
    expect(tag.getAttribute('data-state')).toBe('unchecked');
    expect(associateQuoteClient).not.toHaveBeenCalled();
  });

  // The contact hint arrives as E.164 and the picker splits it; a number no country can read blocks.
  it('saves a new client with the hinted phone, and refuses one that is not a phone', async () => {
    const copy = messages.clients.association.dialog;
    const view = renderCard();
    await view.findByText(messages.clients.association.pending.title);
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));
    fireEvent.click(await view.findByRole('radio', { name: copy.new }));

    const phone = view.getByLabelText(copy.phone) as HTMLInputElement;
    expect(phone.value).toBe('91155550101');

    fireEvent.change(phone, { target: { value: '1234' } });
    expect(view.getByText(messages.common.phone.invalid)).toBeTruthy();
    expect(view.getByRole('button', { name: copy.save }).hasAttribute('disabled')).toBe(true);

    fireEvent.change(phone, { target: { value: '11 15 5555-0101' } });
    fireEvent.click(view.getByRole('button', { name: copy.save }));

    await waitFor(() =>
      expect(associateQuoteClient).toHaveBeenCalledWith(
        QUOTE_ID,
        BRANCH_ID,
        expect.objectContaining({
          new_client: expect.objectContaining({ phone: '+5491155550101' }),
        }),
      ),
    );
  });

  it('lets the seller manually select an existing client without a contact match', async () => {
    const manualClient: ClientMatch = {
      client: {
        id: 'c0000000-0000-4000-8000-000000000023',
        name: 'Constructora del Sur',
        phone: '+5491144440101',
        email: 'administracion@delsur.test',
        originChannel: 'WHATSAPP',
        notes: null,
        createdAt: NOW,
        updatedAt: NOW,
      },
      tags: [{ id: TAG_ID, name: 'Recurrente', createdAt: NOW }],
    };
    vi.mocked(getQuoteClientAssociation).mockResolvedValue({
      ...association,
      suggestions: [],
    });
    vi.mocked(getClientDirectory).mockResolvedValue([manualClient]);

    const view = renderCard();
    await view.findByText(messages.clients.association.pending.title);
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));

    const search = await view.findByPlaceholderText(
      messages.clients.association.dialog.searchPlaceholder,
    );
    fireEvent.change(search, { target: { value: '4444' } });
    fireEvent.click(await view.findByRole('radio', { name: /Constructora del Sur/ }));
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.dialog.save }));

    await waitFor(() =>
      expect(associateQuoteClient).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID, {
        client_id: manualClient.client.id,
        tag_ids: [TAG_ID],
      }),
    );
  });

  it('can create a reusable tag inline without associating the sale', async () => {
    vi.mocked(createClientTag).mockResolvedValue({
      id: 'a0000000-0000-4000-8000-000000000023',
      name: 'Mayorista',
      createdAt: NOW,
    });
    const view = renderCard();
    await view.findByText(messages.clients.association.pending.title);
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.associate }));

    fireEvent.change(view.getByPlaceholderText(messages.clients.association.tags.newPlaceholder), {
      target: { value: 'Mayorista' },
    });
    fireEvent.click(view.getByRole('button', { name: messages.clients.association.tags.create }));

    await waitFor(() => expect(createClientTag).toHaveBeenCalledWith('Mayorista'));
    expect(await view.findByRole('checkbox', { name: 'Mayorista' })).toBeTruthy();
    expect(associateQuoteClient).not.toHaveBeenCalled();
  });
});
