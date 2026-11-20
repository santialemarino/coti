import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ClientMergeDialog } from '@/app/(protected)/clients/[id]/_components/client-merge-dialog';
import type { Client } from '@/lib/api/client-profiles';
import { getMergeCandidates, mergeClient } from '@/lib/api/clients-client';
import messages from '@/translations/es.json';

const refresh = vi.fn();
const push = vi.fn();
vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ refresh }),
}));
vi.mock('@/hooks/use-pending-router', () => ({ usePendingRouter: () => ({ push }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({ getMergeCandidates: vi.fn(), mergeClient: vi.fn() }));
const { toast } = await import('sonner');
const t = messages.clients.merge;

function candidate(overrides: Partial<Client>): Client {
  return {
    id: 'other',
    taxId: null,
    name: 'Juan WhatsApp',
    phone: '+5491155550101',
    email: null,
    originChannel: null,
    notes: null,
    createdAt: '2026-11-01T00:00:00Z',
    updatedAt: '2026-11-01T00:00:00Z',
    ...overrides,
  } as Client;
}

async function pick(taxId: string | null, other: Client) {
  vi.mocked(getMergeCandidates).mockResolvedValue([other]);
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientMergeDialog clientId="this" name="Juan Web" taxId={taxId} />
    </NextIntlClientProvider>,
  );
  fireEvent.click(view.getByRole('button', { name: t.open }));
  fireEvent.click(await view.findByRole('combobox', { name: t.other.label }));
  fireEvent.click(await view.findByRole('option', { name: new RegExp(other.name ?? '') }));
  return view;
}

beforeEach(() => vi.clearAllMocks());

describe('ClientMergeDialog', () => {
  it('keeps the profile with a CUIT without asking, and says the portal waits for proof', async () => {
    vi.mocked(mergeClient).mockResolvedValue('this');
    const view = await pick('20123456786', candidate({}));

    expect(view.queryByRole('radio')).toBeNull();
    expect(view.getByText(t.portalHold.replace('{kept}', 'Juan Web'))).toBeTruthy();
    fireEvent.click(view.getByRole('button', { name: t.submit }));
    await waitFor(() => expect(mergeClient).toHaveBeenCalledWith('this', 'other'));
    expect(toast.success).toHaveBeenCalledWith(
      t.merged.replace('{merged}', 'Juan WhatsApp').replace('{kept}', 'Juan Web'),
    );
    expect(refresh).toHaveBeenCalled();
  });

  it('shows the other profile’s CUIT and keeps it when only it has one', async () => {
    vi.mocked(mergeClient).mockResolvedValue('other');
    const other = candidate({ taxId: '20123456786', name: 'Juan Cuenta' });
    const view = await pick(null, other);

    fireEvent.click(view.getByRole('button', { name: t.submit }));
    await waitFor(() => expect(mergeClient).toHaveBeenCalledWith('other', 'this'));
    expect(push).toHaveBeenCalledWith(expect.stringContaining('other'));
  });

  it('follows the survivor the API names, not the radio', async () => {
    vi.mocked(mergeClient).mockResolvedValue('other');
    const view = await pick(null, candidate({}));

    expect(view.getAllByRole('radio')).toHaveLength(2);
    fireEvent.click(view.getByRole('button', { name: t.submit }));
    await waitFor(() => expect(mergeClient).toHaveBeenCalledWith('this', 'other'));
    expect(push).toHaveBeenCalledWith(expect.stringContaining('other'));
    expect(toast.success).toHaveBeenCalledWith(
      t.merged.replace('{merged}', 'Juan Web').replace('{kept}', 'Juan WhatsApp'),
    );
  });
});
