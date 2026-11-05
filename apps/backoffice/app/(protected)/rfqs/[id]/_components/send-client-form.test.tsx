import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ClientMatch, QuoteClientAssociation } from '@/lib/api/client-profiles';
import { getClientDirectory, getQuoteClientAssociation } from '@/lib/api/clients-client';
import messages from '@/translations/es.json';
import { SendClientForm } from './send-client-form';

vi.mock('@/lib/api/clients-client', () => ({
  getClientDirectory: vi.fn(),
  getQuoteClientAssociation: vi.fn(),
}));
const copy = messages.rfqs.detail.send;
const taxId = '20123456786';
const candidate: ClientMatch = {
  client: {
    id: 'client-1',
    taxId,
    name: 'Existing company',
    email: null,
    phone: null,
    notes: null,
    originChannel: null,
    createdAt: '2026-10-19T00:00:00Z',
    updatedAt: '2026-10-19T00:00:00Z',
  },
  tags: [],
};
const context: QuoteClientAssociation = {
  currentClient: null,
  suggestions: [],
  availableTags: [],
  contactHints: { taxId, name: 'Web customer', phone: null, email: 'customer@example.test' },
};

function renderForm() {
  const onChange = vi.fn();
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <SendClientForm quoteId="quote-1" branchId="branch-1" disabled={false} onChange={onChange} />
    </NextIntlClientProvider>,
  );
  return { ...view, onChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getQuoteClientAssociation).mockResolvedValue(context);
  vi.mocked(getClientDirectory).mockResolvedValue([]);
});

describe('send client selection', () => {
  it('prefills a new profile from the web request without saving it separately', async () => {
    const view = renderForm();
    await waitFor(() =>
      expect(view.onChange).toHaveBeenLastCalledWith({
        taxId: '',
        newClient: { name: 'Web customer', email: 'customer@example.test' },
        tagIds: [],
      }),
    );
    expect((view.getByLabelText(new RegExp(copy.clientName)) as HTMLInputElement).value).toBe(
      'Web customer',
    );
    expect(getQuoteClientAssociation).toHaveBeenCalledWith('quote-1', 'branch-1');
  });
  it('keeps the current registered profile and its tags', async () => {
    const tag = { id: 'tag-1', name: 'Frecuente', createdAt: '2026-10-19T00:00:00Z' };
    vi.mocked(getQuoteClientAssociation).mockResolvedValue({
      ...context,
      currentClient: { ...candidate, tags: [tag] },
    });
    const view = renderForm();
    await waitFor(() =>
      expect(view.onChange).toHaveBeenLastCalledWith({
        taxId,
        clientId: 'client-1',
        tagIds: ['tag-1'],
      }),
    );
    expect(view.queryByLabelText(new RegExp(copy.clientName))).toBeNull();
  });
  it('does not claim a fiscal profile from unverified request declarations', async () => {
    vi.mocked(getClientDirectory).mockResolvedValue([candidate]);
    const view = renderForm();
    await waitFor(() =>
      expect(view.onChange).toHaveBeenLastCalledWith({
        taxId: '',
        newClient: { name: 'Web customer', email: 'customer@example.test' },
        tagIds: [],
      }),
    );
    expect(view.queryByLabelText(new RegExp(copy.clientTax))).toBeNull();
  });
  it('requires a contact for a new provisional profile', async () => {
    const view = renderForm();
    await waitFor(() =>
      expect(view.onChange).toHaveBeenLastCalledWith(
        expect.objectContaining({ newClient: expect.anything() }),
      ),
    );
    fireEvent.change(view.getByLabelText(copy.clientEmail), { target: { value: '' } });
    await waitFor(() => expect(view.onChange).toHaveBeenLastCalledWith(null));
  });
  it('leaves sending unavailable when client context cannot be read', async () => {
    vi.mocked(getQuoteClientAssociation).mockRejectedValue(new Error('offline'));
    const view = renderForm();
    await view.findByText(copy.clientLoadFailed);
    expect(view.onChange).toHaveBeenLastCalledWith(null);
  });
});
