import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ClientContactSuggestionDialog } from '@/app/(protected)/clients/_components/client-contact-suggestion-dialog';
import type { ContactSuggestion } from '@/lib/api/client-contact-suggestions';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(() => vi.unstubAllGlobals());
const t = messages.clientContactSuggestions;

const CASE: ContactSuggestion = {
  id: 'case',
  sourceClientId: 'loser',
  candidateClientId: 'owner',
  candidateName: 'Owner',
  sourceName: 'Loser',
  sourceRegistered: false,
  candidateRegistered: false,
  taxId: null,
  kind: 'EMAIL',
  value: 'shared@example.test',
  declaredName: null,
  originalText: 'shared@example.test',
  createdAt: 'date',
};

function show(suggestion: ContactSuggestion, onResolved = vi.fn()) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientContactSuggestionDialog
        suggestion={suggestion}
        branchId="branch"
        onClose={vi.fn()}
        onResolved={onResolved}
      />
    </NextIntlClientProvider>,
  );
}

describe('ClientContactSuggestionDialog', () => {
  it('merges into the profile the seller keeps', async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetcher);
    const onResolved = vi.fn();
    const view = show(CASE, onResolved);

    fireEvent.click(view.getByRole('radio', { name: 'Loser' }));
    fireEvent.click(view.getByRole('button', { name: t.merge }));

    await waitFor(() => expect(onResolved).toHaveBeenCalled());
    const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/client-contact-suggestions/case');
    expect(JSON.parse(init.body as string)).toEqual({ action: 'MERGE', keep_client_id: 'loser' });
  });

  // A profile with a CUIT is merged only by its customer, so the seller can only dismiss the case.
  it('offers only a dismissal when either profile is registered', async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetcher);
    const view = show({ ...CASE, candidateRegistered: true });

    expect(view.queryByRole('button', { name: t.merge })).toBeNull();
    expect(view.getByText(t.registered)).toBeTruthy();
    fireEvent.click(view.getByRole('button', { name: t.separate }));

    await waitFor(() => expect(fetcher).toHaveBeenCalled());
    const [, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toEqual({ action: 'SEPARATE' });
  });
});
