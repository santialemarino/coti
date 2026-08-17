import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { isMessageShown } from '@repo/vitest-config/form-messages';
import { ForgotPasswordForm } from '@/app/(auth)/forgot-password/_components/forgot-password-form';
import messages from '@/translations/es.json';

vi.mock('@/app/(auth)/forgot-password/actions', () => ({ requestPasswordRecovery: vi.fn() }));

const { requestPasswordRecovery } = await import('@/app/(auth)/forgot-password/actions');

const copy = messages.auth.forgotPassword;

// The real catalog, so a screen bound to the wrong namespace fails here rather than quietly
// resolving to a sentence that belongs to another flow.
function renderForm() {
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ForgotPasswordForm />
    </NextIntlClientProvider>,
  );
  const submit = view.container.querySelector('button[type="submit"]');
  const email = view.container.querySelector('input[name="email"]');
  if (!(submit instanceof HTMLButtonElement)) throw new Error('no submit button rendered');
  if (!(email instanceof HTMLInputElement)) throw new Error('no email field rendered');
  return { ...view, submit, email };
}

function request(view: ReturnType<typeof renderForm>) {
  fireEvent.change(view.email, { target: { value: 'ana@corralonsanmartin.test' } });
  fireEvent.click(view.submit);
}

beforeEach(() => vi.clearAllMocks());

describe('ForgotPasswordForm', () => {
  it('replaces the card with the notice once the request is accepted', async () => {
    vi.mocked(requestPasswordRecovery).mockResolvedValue({ sent: true });
    const view = renderForm();

    request(view);

    await waitFor(() => expect(view.getByText(copy.sent)).toBeTruthy());
  });

  /*
   * The audit's headline case: the caller-keyed mail allowance answers 429, and the screen used to
   * read it as "Ocurrió un error inesperado" — a message that says nothing about waiting.
   */
  it('names the allowance running out instead of calling it unexpected', async () => {
    vi.mocked(requestPasswordRecovery).mockResolvedValue({ error: 'RATE_LIMITED' });
    const view = renderForm();

    request(view);

    await waitFor(() => expect(view.getByText(messages.errors.RATE_LIMITED)).toBeTruthy());
    expect(view.queryByText(messages.errors.INTERNAL)).toBeNull();
  });

  // The address is the only field there is, so a rejected body belongs on it and clears as the
  // caller edits it.
  it('lands a rejected address on the field and clears it on the next keystroke', async () => {
    vi.mocked(requestPasswordRecovery).mockResolvedValue({
      error: 'INVALID_BODY',
      field: 'email',
    });
    const view = renderForm();

    request(view);

    await waitFor(() =>
      expect(isMessageShown(view.container, copy.errors.INVALID_BODY)).toBe(true),
    );
    expect(view.email.getAttribute('aria-invalid')).toBe('true');

    fireEvent.change(view.email, { target: { value: 'ana@otro.test' } });
    await waitFor(() =>
      expect(isMessageShown(view.container, copy.errors.INVALID_BODY)).toBe(false),
    );
  });

  it('falls back to the flow wording when the failure carried no code', async () => {
    vi.mocked(requestPasswordRecovery).mockResolvedValue({});
    const view = renderForm();

    request(view);

    await waitFor(() => expect(view.getByText(copy.errors.INTERNAL)).toBeTruthy());
  });
});
