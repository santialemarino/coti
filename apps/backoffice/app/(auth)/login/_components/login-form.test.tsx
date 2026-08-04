import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { LoginForm } from '@/app/(auth)/login/_components/login-form';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
}));
vi.mock('@/app/(auth)/login/actions', () => ({ login: vi.fn() }));

const { login } = await import('@/app/(auth)/login/actions');

const copy = messages.auth.login;

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderLogin() {
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <LoginForm />
    </NextIntlClientProvider>,
  );
  // By type, not by accessible name: the password field's reveal toggle is a button too, and
  // the submit button's own name changes the moment it goes pending.
  const submit = view.container.querySelector('button[type="submit"]');
  if (!(submit instanceof HTMLButtonElement)) throw new Error('no submit button rendered');
  return { ...view, submit };
}

/*
 * By registered name, because the password label is also the reveal toggle's accessible name and
 * a label query matches both. fireEvent rather than assigning .value, which React never sees —
 * that leaves the field empty, validation failing, and the submit this test is about never starts.
 */
function fillCredentials(container: HTMLElement) {
  const field = (name: string) => {
    const input = container.querySelector(`input[name="${name}"]`);
    if (!(input instanceof HTMLInputElement)) throw new Error(`no ${name} field rendered`);
    return input;
  };

  fireEvent.change(field('email'), { target: { value: 'admin@corralonsanmartin.test' } });
  fireEvent.change(field('password'), { target: { value: 'coti1234' } });
}

beforeEach(() => vi.clearAllMocks());

describe('LoginForm submit button', () => {
  it('renders its idle label and is operable', () => {
    const { submit } = renderLogin();

    expect(submit.textContent).toContain(copy.submit);
    expect(submit.disabled).toBe(false);
    expect(submit.getAttribute('aria-busy')).toBe('false');
  });

  /*
   * `pending` is optional on PendingButton and defaults to false, so a form that passes
   * `pendingLabel` and forgets to wire `pending` type-checks cleanly and silently never shows
   * the state. That is the failure this pins, for the whole family of converted forms.
   */
  it('goes busy and disables itself while the action is in flight', async () => {
    let release = () => {};
    vi.mocked(login).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ redirectTo: '/' }))),
    );

    const { container, submit } = renderLogin();
    fillCredentials(container);
    fireEvent.click(submit);

    await waitFor(() => expect(submit.getAttribute('aria-busy')).toBe('true'));
    expect(submit.disabled).toBe(true);
    expect(submit.textContent).toContain(copy.submitting);

    release();
    await waitFor(() => expect(login).toHaveBeenCalledOnce());
  });
});
