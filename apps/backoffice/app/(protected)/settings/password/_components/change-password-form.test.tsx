import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ChangePasswordForm } from '@/app/(protected)/settings/password/_components/change-password-form';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/password/actions', () => ({ changePassword: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { changePassword } = await import('@/app/(protected)/settings/password/actions');
const { toast } = await import('sonner');

const copy = messages.auth.changePassword;

const VALUES = {
  currentPassword: 'Coti-1234-vieja',
  newPassword: 'Coti-1234-nueva',
  confirmPassword: 'Coti-1234-nueva',
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ChangePasswordForm />
    </NextIntlClientProvider>,
  );
}

/*
 * By registered name rather than by label: a password label is also its reveal toggle's accessible
 * name. fireEvent.change rather than assigning .value, which React never sees.
 */
function fill(view: RenderResult, values: Record<string, string>) {
  Object.entries(values).forEach(([name, value]) => {
    const input = view.baseElement.querySelector(`input[name="${name}"]`);
    if (!(input instanceof HTMLInputElement)) throw new Error(`no ${name} field on screen`);
    fireEvent.change(input, { target: { value } });
  });
}

function values(view: RenderResult): string[] {
  return [...view.baseElement.querySelectorAll('input[name]')].map(
    (i) => (i as HTMLInputElement).value,
  );
}

// Clicking a submit button does not reach a React form in jsdom, which implements no requestSubmit.
function submit(view: RenderResult) {
  const form = view.baseElement.querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form on screen');
  fireEvent.submit(form);
}

beforeEach(() => vi.clearAllMocks());

describe('ChangePasswordForm', () => {
  /*
   * A confirmation of something just done is transient, so it is a toast — a standing paragraph
   * outlives the change it reports and is still there for the next attempt.
   */
  it('confirms a change with a toast and leaves nothing standing on the page', async () => {
    vi.mocked(changePassword).mockResolvedValue({ done: true });
    const view = renderForm();

    fill(view, VALUES);
    submit(view);

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.done));
    expect(view.queryByText(copy.done)).toBeNull();
  });

  // A password should not sit in the DOM once it has been used, and there is nothing to resubmit.
  it('empties every field once the password is changed', async () => {
    vi.mocked(changePassword).mockResolvedValue({ done: true });
    const view = renderForm();

    fill(view, VALUES);
    submit(view);

    await waitFor(() => expect(changePassword).toHaveBeenCalledWith(VALUES));
    await waitFor(() => expect(values(view)).toEqual(['', '', '']));
  });

  // The wrong current password is the caller's to fix, so it lands on the field they will edit.
  it('puts a refused current password on that field', async () => {
    vi.mocked(changePassword).mockResolvedValue({
      error: 'UNAUTHENTICATED',
      field: 'currentPassword',
    });
    const view = renderForm();

    fill(view, VALUES);
    submit(view);

    await waitFor(() => expect(view.getByText(copy.errors.UNAUTHENTICATED)).toBeTruthy());
    expect(toast.success).not.toHaveBeenCalled();
  });

  /*
   * `PendingButton`'s `pending` prop is optional and defaults to false, so a form that passes the
   * label and forgets to wire the state type-checks cleanly and silently never shows it.
   */
  it('shows the pending state on the submit button', async () => {
    let release = () => {};
    vi.mocked(changePassword).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ done: true }))),
    );
    const view = renderForm();
    // Held by node, not re-queried by name: both labels are on it while they crossfade.
    const button = view.baseElement.querySelector('button[type="submit"]');
    if (!(button instanceof HTMLButtonElement)) throw new Error('no submit button on screen');

    fill(view, VALUES);
    submit(view);

    await waitFor(() => expect(button.getAttribute('aria-busy')).toBe('true'));
    expect(button).toHaveProperty('disabled', true);
    expect(button.textContent).toContain(copy.submitting);

    release();
    await waitFor(() => expect(changePassword).toHaveBeenCalledOnce());
  });
});
