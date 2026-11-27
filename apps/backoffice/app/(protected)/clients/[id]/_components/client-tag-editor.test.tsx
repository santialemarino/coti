import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it, vi } from 'vitest';

import { replaceClientTags, setClientTagExclusion } from '@/lib/api/clients-client';
import messages from '@/translations/es.json';
import { ClientTagEditor } from './client-tag-editor';

const refresh = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({
  createClientTag: vi.fn(),
  replaceClientTags: vi.fn(),
  setClientTagExclusion: vi.fn(),
}));

it('saves manual tags separately and toggles automatic exclusions', async () => {
  vi.mocked(replaceClientTags).mockResolvedValue([]);
  vi.mocked(setClientTagExclusion).mockResolvedValue(undefined);
  const manual = { id: 'manual', name: 'Constructora', createdAt: '' };
  const automatic = { id: 'recurring', name: 'Recurrente', createdAt: '', automatic: true };
  const renderEditor = (excluded: boolean) => (
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientTagEditor
        clientId="client"
        initialTags={excluded ? [manual] : [manual, automatic]}
        initialAvailable={[manual]}
        excludedTags={excluded ? [automatic] : []}
      />
    </NextIntlClientProvider>
  );
  const view = render(renderEditor(false));

  fireEvent.click(view.getByRole('button', { name: messages.clients.tags.save }));
  await waitFor(() => expect(replaceClientTags).toHaveBeenCalledWith('client', ['manual']));

  const exclude = view.getByRole('button', { name: 'Quitar Recurrente' });
  await waitFor(() => expect(exclude).toHaveProperty('disabled', false));
  fireEvent.click(exclude);
  await waitFor(() =>
    expect(setClientTagExclusion).toHaveBeenCalledWith('client', 'recurring', true),
  );
  await waitFor(() => expect(refresh).toHaveBeenCalled());

  view.rerender(renderEditor(true));
  const restore = view.getByRole('button', { name: 'Volver a calcular Recurrente' });
  await waitFor(() => expect(restore).toHaveProperty('disabled', false));
  fireEvent.click(restore);
  await waitFor(() =>
    expect(setClientTagExclusion).toHaveBeenLastCalledWith('client', 'recurring', false),
  );
});
