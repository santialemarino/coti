import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, expect, it, vi } from 'vitest';

import { ClientContactSuggestions } from '@/app/(protected)/clients/_components/client-contact-suggestions';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ usePathname: () => '/clients/suggestions' }));
vi.mock('@/app/(protected)/rfqs/_components/rfq-list-context', () => ({
  useRfqList: () => ({ activeBranchId: 'branch' }),
}));
afterEach(() => vi.unstubAllGlobals());

function renderSuggestions(compact = false, initialItems?: []) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ClientContactSuggestions compact={compact} initialItems={initialItems} />
    </NextIntlClientProvider>,
  );
}

// A list of unknown length has no placeholder that matches it, so the column waits empty.
it('shows no placeholder in the column while its first load is in flight', () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise(() => {})),
  );
  const view = renderSuggestions(true);
  expect(view.container.querySelector('[data-slot="skeleton"]')).toBeNull();
});

it('shows a server-resolved empty rail immediately while refreshing in the background', () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise(() => {})),
  );
  renderSuggestions(true, []);
  expect(screen.getByText(messages.clientContactSuggestions.empty)).toBeTruthy();
  expect(screen.queryByRole('link', { name: /0 coincidencias/ })).toBeNull();
});

it('keeps pending suggestions reachable from the contextual column', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify([
            {
              id: 'case',
              source_client_id: null,
              candidate_client_id: 'client',
              candidate_name: 'Customer',
              tax_id: '20123456786',
              kind: 'EMAIL',
              value: 'new@example.test',
              declared_name: null,
              original_text: 'Original',
              created_at: 'date',
            },
          ]),
          { status: 200 },
        ),
    ),
  );
  renderSuggestions(true);
  const link = await screen.findByRole('link', { name: '1 coincidencia pendiente' });
  expect(link.getAttribute('href')).toBe('/clients/suggestions');
  expect(screen.getByRole('link', { name: 'Customer' }).getAttribute('href')).toBe(
    '/clients/suggestions#case',
  );
});

it('distinguishes load failures from empty pending lists and retries', async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(new Response('', { status: 500 }))
    .mockResolvedValue(new Response('[]', { status: 200 }));
  vi.stubGlobal('fetch', fetcher);
  renderSuggestions();
  await screen.findByRole('alert');
  fireEvent.click(screen.getByRole('button', { name: messages.clientContactSuggestions.retry }));
  await waitFor(() =>
    expect(screen.getByText(messages.clientContactSuggestions.empty)).toBeDefined(),
  );
});
