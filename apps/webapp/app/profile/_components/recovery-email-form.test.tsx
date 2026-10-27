import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { RecoveryEmailForm } from '@/app/profile/_components/recovery-email-form';
import { saveRecoveryEmail } from '@/app/profile/actions';
import messages from '@/translations/es.json';

const mocks = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: mocks.refresh }) }));
vi.mock('@/app/profile/actions', () => ({ saveRecoveryEmail: vi.fn(async () => ({})) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
beforeEach(() => vi.clearAllMocks());

function renderForm(email: string | null = null, verified = false) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RecoveryEmailForm recovery={{ email, verified }} />
    </NextIntlClientProvider>,
  );
}

it('explains phone-only recovery and validates the optional address', async () => {
  renderForm();
  const t = messages.customerProfile.recovery;
  expect(screen.getByText(t.missing)).toBeDefined();
  fireEvent.change(screen.getByLabelText(t.email), { target: { value: 'invalid' } });
  fireEvent.click(screen.getByRole('button', { name: t.save }));
  await screen.findByText(t.invalid);
  expect(saveRecoveryEmail).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText(t.email), { target: { value: 'recovery@example.test' } });
  fireEvent.click(screen.getByRole('button', { name: t.save }));
  await waitFor(() => expect(saveRecoveryEmail).toHaveBeenCalledWith('recovery@example.test'));
  expect(mocks.refresh).toHaveBeenCalled();
});

it('resends a pending proof and permits clearing a verified address', async () => {
  const view = renderForm('pending@example.test');
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.recovery.resend }));
  await waitFor(() => expect(saveRecoveryEmail).toHaveBeenCalledWith('pending@example.test'));
  view.unmount();
  renderForm('verified@example.test', true);
  expect(
    screen
      .getByRole('button', { name: messages.customerProfile.recovery.save })
      .hasAttribute('disabled'),
  ).toBe(true);
  fireEvent.change(screen.getByLabelText(messages.customerProfile.recovery.email), {
    target: { value: '' },
  });
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.recovery.save }));
  await waitFor(() => expect(saveRecoveryEmail).toHaveBeenCalledWith(''));
});
