import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { QuoteClientAssociation } from '@/lib/api/client-profiles';
import {
  associateQuoteClient,
  createClientTag,
  getQuoteClientAssociation,
} from '@/lib/api/clients-client';
import messages from '@/translations/es.json';
import { ClientAssociationCard } from './client-association-card';

vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({
  associateQuoteClient: vi.fn(),
  createClientTag: vi.fn(),
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

function renderCard() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ClientAssociationCard quoteId={QUOTE_ID} branchId={BRANCH_ID} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getQuoteClientAssociation).mockResolvedValue(association);
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
