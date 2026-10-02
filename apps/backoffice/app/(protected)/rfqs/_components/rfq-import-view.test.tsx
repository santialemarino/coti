import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqImportView } from '@/app/(protected)/rfqs/_components/rfq-import-view';
import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import type { Channel } from '@/lib/api/channels';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  unstable_rethrow: vi.fn(),
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
vi.mock('@/lib/api/channels', () => ({ listChannels: vi.fn() }));
vi.mock('@/lib/api/rfqs-client', () => ({ createFileRfqDraft: vi.fn() }));

const { listChannels } = await import('@/lib/api/channels');

const WHATSAPP: Channel = { id: 'c-wa', type: 'WHATSAPP', identifier: '+5491122542609' };
const EMAIL: Channel = { id: 'c-mail', type: 'EMAIL', identifier: 'moron@corralon.test' };

function view(branchId: string | null) {
  return (
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqListProvider records={[]} activeBranchId={branchId} userName="Admin" userId="u1" isAdmin>
        <RfqImportView
          onBack={vi.fn()}
          onClose={vi.fn()}
          onCreated={vi.fn()}
          activeBranchId={branchId}
        />
      </RfqListProvider>
    </NextIntlClientProvider>
  );
}

function renderView(branchId: string | null = 'b1') {
  return render(view(branchId));
}

describe('RfqImportView channel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('settles a sole channel instead of offering a choice of one', async () => {
    vi.mocked(listChannels).mockResolvedValue([WHATSAPP]);
    renderView();

    expect(await screen.findByText('Es el único canal abierto de la sucursal.')).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: '¿Por dónde llegó?' })).toBeNull();
  });

  // The first of several is a guess the seller would have to notice and undo.
  it('preselects nothing when the branch has several channels', async () => {
    vi.mocked(listChannels).mockResolvedValue([WHATSAPP, EMAIL]);
    renderView();

    const picker = await screen.findByRole('combobox', { name: '¿Por dónde llegó?' });
    expect(picker.textContent).toBe('Elegí el canal');
  });

  // A channel settled for one branch is not a channel of the next one: kept, it would still be the
  // one submitted, while the picker shows nothing chosen.
  it('drops the channel when the branch changes', async () => {
    const OTHER: Channel = { id: 'c-other', type: 'WHATSAPP', identifier: '+5491100000000' };
    vi.mocked(listChannels).mockImplementation(async (branchId) =>
      branchId === 'b1' ? [WHATSAPP] : [OTHER, EMAIL],
    );
    const { container, rerender } = renderView('b1');
    await screen.findByText('Es el único canal abierto de la sucursal.');
    const input = container.querySelector('input[type="file"]');
    if (!(input instanceof HTMLInputElement)) throw new Error('no file input');
    fireEvent.change(input, { target: { files: [new File(['x'], 'pedido.pdf')] } });
    const submit = screen.getByRole('button', { name: messages.rfqs.create.import.submit });
    expect((submit as HTMLButtonElement).disabled).toBe(false);

    rerender(view('b2'));

    await screen.findByRole('combobox', { name: '¿Por dónde llegó?' });
    expect((submit as HTMLButtonElement).disabled).toBe(true);
  });

  it('explains a branch with no open channel', async () => {
    vi.mocked(listChannels).mockResolvedValue([]);
    renderView();

    expect(await screen.findByText('Esta sucursal no tiene canales abiertos')).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: '¿Por dónde llegó?' })).toBeNull();
  });

  it('says the channels could not be loaded rather than that there are none', async () => {
    vi.mocked(listChannels).mockRejectedValue(new Error('GET /api/channels answered 500'));
    renderView();

    expect(await screen.findByText(/No pudimos cargar los canales/)).toBeTruthy();
    expect(screen.queryByText('Esta sucursal no tiene canales abiertos')).toBeNull();
  });
});
