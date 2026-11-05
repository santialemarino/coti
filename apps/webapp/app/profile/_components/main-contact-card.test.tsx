import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { MainContactCard } from '@/app/profile/_components/main-contact-card';
import { chooseMainContact } from '@/app/profile/actions';
import messages from '@/translations/es.json';

const mocks = vi.hoisted(() => ({ refresh: vi.fn(), success: vi.fn(), error: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: mocks.refresh }) }));
vi.mock('sonner', () => ({ toast: { success: mocks.success, error: mocks.error } }));
vi.mock('@/app/profile/actions', () => ({ chooseMainContact: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

function show() {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <MainContactCard
        contacts={[
          { kind: 'EMAIL', value: 'main@example.test', primary: true, verified: true, main: true },
          { kind: 'EMAIL', value: 'other@example.test', primary: false, verified: true },
          { kind: 'EMAIL', value: 'pending@example.test', primary: false, verified: false },
        ]}
      />
    </NextIntlClientProvider>,
  );
}

it('offers only verified contacts and starts on the main one', () => {
  show();
  expect(screen.getAllByRole('radio')).toHaveLength(2);
  expect(screen.queryByText(/pending@example\.test/)).toBeNull();
  expect(screen.getByRole('radio', { name: /main@example/ }).getAttribute('aria-checked')).toBe(
    'true',
  );
});

it('saves the chosen contact and goes back to the saved one when refused', async () => {
  vi.mocked(chooseMainContact).mockResolvedValue({ error: 'refused' });
  show();
  fireEvent.click(screen.getByRole('radio', { name: /other@example/ }));
  await waitFor(() =>
    expect(chooseMainContact).toHaveBeenCalledWith('EMAIL', 'other@example.test'),
  );
  await waitFor(() => expect(mocks.error).toHaveBeenCalledWith('refused'));
  expect(screen.getByRole('radio', { name: /main@example/ }).getAttribute('aria-checked')).toBe(
    'true',
  );
});
