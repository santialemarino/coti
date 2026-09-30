import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { ClientTable } from '@/app/(protected)/clients/_components/client-table';
import type { ClientSummary } from '@/lib/api/client-profiles';
import messages from '@/translations/es.json';

const CLIENT: ClientSummary = {
  id: 'c0000000-0000-4000-8000-000000000001',
  name: 'Constructora del Oeste',
  phone: '+5491155550002',
  email: null,
  originChannel: 'WHATSAPP',
  notes: null,
  createdAt: '2026-09-01T12:00:00Z',
  updatedAt: '2026-09-01T12:00:00Z',
  tags: [],
  acceptedQuoteCount: 1,
  lastAcceptedAt: '2026-09-29T12:00:00Z',
};

function renderTable() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ClientTable clients={[CLIENT]} />
    </NextIntlClientProvider>,
  );
}

describe('ClientTable', () => {
  // The page's heading already says "Clientes"; the card names what it lists instead of repeating it.
  it('titles its card apart from the page heading', () => {
    const view = renderTable();

    expect(view.getByText(messages.clients.table.title)).toBeTruthy();
    expect(
      view.queryByText(messages.clients.title, { selector: '[data-slot=card-title]' }),
    ).toBeNull();
  });

  it('opens a profile through its route', () => {
    const view = renderTable();

    const link = view.getByRole('link', { name: new RegExp(messages.clients.viewProfile) });
    expect(link.getAttribute('href')).toBe(`/clients/${CLIENT.id}`);
  });
});
