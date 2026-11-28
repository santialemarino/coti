import { fireEvent, render, waitFor } from '@testing-library/react';
import { useReducedMotion } from 'motion/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { TooltipProvider } from '@repo/ui/components';
import { ClientNotesEditor } from '@/components/client-notes-editor';
import { updateClientNotes } from '@/lib/api/clients-client';
import { ApiError } from '@/lib/api/errors';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ refresh }),
}));
vi.mock('motion/react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('motion/react')>()),
  useReducedMotion: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/lib/api/clients-client', () => ({ updateClientNotes: vi.fn() }));
const copy = messages.clients.notes;

function show(notes: string | null) {
  return render(
    <NextIntlClientProvider locale="es-AR" messages={messages}>
      <TooltipProvider>
        <ClientNotesEditor clientId="client-1" name="Juan" notes={notes} />
      </TooltipProvider>
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useReducedMotion).mockReturnValue(false);
});

it.each([false, true])(
  'cancels on Escape and restores focus with reduced motion=%s',
  async (reduced) => {
    vi.mocked(useReducedMotion).mockReturnValue(reduced);
    const view = show('Existing note');
    fireEvent.click(view.getByRole('button', { name: copy.edit }));
    const input = await view.findByRole('textbox', { name: copy.label });
    expect(document.activeElement).toBe(input);
    fireEvent.change(input, { target: { value: 'Unsubmitted edit' } });
    fireEvent.keyDown(input, { key: 'Escape' });
    const edit = await view.findByRole('button', { name: copy.edit });
    await waitFor(() => expect(document.activeElement).toBe(edit));
    expect(view.getByText('Existing note')).toBeTruthy();
    expect(updateClientNotes).not.toHaveBeenCalled();
  },
);

it('displays the saved note and refreshes the workspace data', async () => {
  vi.mocked(updateClientNotes).mockResolvedValue({ notes: 'Side entrance' } as never);
  const view = show(null);
  expect(view.getByText(copy.empty)).toBeTruthy();
  fireEvent.click(view.getByRole('button', { name: copy.edit }));
  fireEvent.change(await view.findByRole('textbox', { name: copy.label }), {
    target: { value: ' Side entrance ' },
  });
  fireEvent.click(view.getByRole('button', { name: copy.save }));
  expect(await view.findByText('Side entrance')).toBeTruthy();
  expect(updateClientNotes).toHaveBeenCalledWith('client-1', 'Side entrance');
  expect(refresh).toHaveBeenCalled();
});

it('keeps the editor open and shows a save error inline', async () => {
  vi.mocked(updateClientNotes).mockRejectedValue(new ApiError('INTERNAL', 500));
  const view = show(null);
  fireEvent.click(view.getByRole('button', { name: copy.edit }));
  fireEvent.change(await view.findByRole('textbox', { name: copy.label }), {
    target: { value: 'Keep typing' },
  });
  fireEvent.click(view.getByRole('button', { name: copy.save }));
  expect(await view.findByText(copy.errors.INTERNAL)).toBeTruthy();
  expect(view.getByRole('textbox', { name: copy.label })).toHaveProperty('value', 'Keep typing');
});
