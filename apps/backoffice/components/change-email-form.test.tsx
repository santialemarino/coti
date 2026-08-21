import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { isMessageShown } from '@repo/vitest-config/form-messages';
import { ChangeEmailForm } from '@/components/change-email-form';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh }) }));
vi.mock('@/lib/auth/change-email', () => ({ changeEmail: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const { changeEmail } = await import('@/lib/auth/change-email');
const { toast } = await import('sonner');

const copy = messages.auth.changeEmail;
const NEW_EMAIL = 'ana.nueva@corralonsanmartin.test';

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm() {
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ChangeEmailForm />
    </NextIntlClientProvider>,
  );
  const submit = view.container.querySelector('button[type="submit"]');
  if (!(submit instanceof HTMLButtonElement)) throw new Error('no submit button rendered');
  return { ...view, submit };
}

/*
 * By registered name: the password label is also the reveal toggle's accessible name, so a label
 * query matches both. fireEvent rather than assigning .value, which React never sees.
 */
function fill(container: HTMLElement, email = NEW_EMAIL) {
  const field = (name: string) => {
    const input = container.querySelector(`input[name="${name}"]`);
    if (!(input instanceof HTMLInputElement)) throw new Error(`no ${name} field rendered`);
    return input;
  };

  fireEvent.change(field('newEmail'), { target: { value: email } });
  fireEvent.change(field('currentPassword'), { target: { value: 'coti1234' } });
}

beforeEach(() => vi.clearAllMocks());

describe('ChangeEmailForm', () => {
  /*
   * The refresh is load-bearing rather than cosmetic: the address it just wrote is confirmed
   * nowhere, so the protected layout's answer changed and only a re-render asks it again. Without
   * this the caller stays on a screen naming the address they replaced.
   */
  it('confirms with the new address and re-renders the tree', async () => {
    vi.mocked(changeEmail).mockResolvedValue({ done: true });
    const { container, submit } = renderForm();
    fill(container);

    fireEvent.click(submit);

    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining(NEW_EMAIL));
  });

  // A password must not sit in the DOM once it has been spent.
  it('clears both fields on success', async () => {
    vi.mocked(changeEmail).mockResolvedValue({ done: true });
    const { container, submit } = renderForm();
    fill(container);

    fireEvent.click(submit);

    await waitFor(() => expect(refresh).toHaveBeenCalled());
    for (const name of ['newEmail', 'currentPassword']) {
      expect(container.querySelector<HTMLInputElement>(`input[name="${name}"]`)?.value).toBe('');
    }
  });

  it('shows the refusal on the field the action names', async () => {
    vi.mocked(changeEmail).mockResolvedValue({ error: 'EMAIL_TAKEN', field: 'newEmail' });
    const { container, submit } = renderForm();
    fill(container);

    fireEvent.click(submit);

    await waitFor(() => expect(isMessageShown(container, copy.errors.EMAIL_TAKEN)).toBe(true));
    expect(refresh).not.toHaveBeenCalled();
  });

  // A refusal belonging to no field still has to be readable, or the button looks inert.
  it('shows a refusal that names no field on the form', async () => {
    vi.mocked(changeEmail).mockResolvedValue({ error: 'INTERNAL' });
    const { container, submit } = renderForm();
    fill(container);

    fireEvent.click(submit);

    await waitFor(() => expect(isMessageShown(container, copy.errors.INTERNAL)).toBe(true));
  });

  // Empty and malformed are different rejections, and the schema must not answer both at once.
  it('refuses a malformed address without calling the action', async () => {
    const { container, submit } = renderForm();
    fill(container, 'no-es-una-direccion');

    fireEvent.click(submit);

    await waitFor(() =>
      expect(isMessageShown(container, messages.common.form.errors.invalidEmail)).toBe(true),
    );
    expect(changeEmail).not.toHaveBeenCalled();
    expect(isMessageShown(container, copy.newEmail.required)).toBe(false);
  });
});
