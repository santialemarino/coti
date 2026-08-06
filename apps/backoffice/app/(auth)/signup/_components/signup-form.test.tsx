import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { isMessageShown } from '@repo/vitest-config/form-messages';
import { SignupForm } from '@/app/(auth)/signup/_components/signup-form';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
}));
vi.mock('@/app/(auth)/signup/actions', () => ({ signup: vi.fn() }));

const { signup } = await import('@/app/(auth)/signup/actions');

const copy = messages.auth.signup;

const ACCOUNT = { accountName: 'Corralón San Martín' };
const BRANCH = { branchName: 'Villa Bosch' };
const ADMIN = {
  adminName: 'Ana Pérez',
  adminEmail: 'ana@corralonsanmartin.test',
  adminPassword: 'Coti-1234-larga',
  confirmPassword: 'Coti-1234-larga',
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderSignup() {
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <SignupForm />
    </NextIntlClientProvider>,
  );
  const form = view.container.querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form rendered');
  return { ...view, form };
}

function fieldOf(view: RenderResult, name: string): HTMLInputElement | null {
  const input = view.container.querySelector(`input[name="${name}"]`);
  return input instanceof HTMLInputElement ? input : null;
}

/*
 * By registered name rather than by label: a password label is also its reveal toggle's
 * accessible name. fireEvent.change rather than assigning .value, which React never sees, so the
 * field would stay empty and the step would never pass its gate.
 */
function fill(view: RenderResult, values: Record<string, string>) {
  Object.entries(values).forEach(([name, value]) => {
    const input = fieldOf(view, name);
    if (!input) throw new Error(`no ${name} field on screen`);
    fireEvent.change(input, { target: { value } });
  });
}

function submitButton(view: RenderResult): HTMLButtonElement {
  const button = view.container.querySelector('button[type="submit"]');
  if (!(button instanceof HTMLButtonElement)) throw new Error('no submit button rendered');
  return button;
}

function backButton(view: RenderResult): HTMLButtonElement {
  const button = view.getByRole('button', { name: copy.back });
  if (!(button instanceof HTMLButtonElement)) throw new Error('no back button rendered');
  return button;
}

// The step swap is a crossfade, so the incoming fields mount only once the outgoing ones leave.
async function waitForStep(view: RenderResult, field: string) {
  await waitFor(() => expect(fieldOf(view, field)).not.toBeNull());
}

async function reachAdminStep(view: RenderResult) {
  fill(view, ACCOUNT);
  fireEvent.submit(view.container.querySelector('form') as HTMLFormElement);
  await waitForStep(view, 'branchName');

  fill(view, BRANCH);
  fireEvent.submit(view.container.querySelector('form') as HTMLFormElement);
  await waitForStep(view, 'adminEmail');
}

beforeEach(() => vi.clearAllMocks());

describe('SignupForm steps', () => {
  it('opens on the account step with only that step on screen', () => {
    const view = renderSignup();

    expect(fieldOf(view, 'accountName')).not.toBeNull();
    expect(fieldOf(view, 'branchName')).toBeNull();
    expect(fieldOf(view, 'adminEmail')).toBeNull();
    expect(submitButton(view).textContent).toContain(copy.next);
  });

  /*
   * A CUIT is written with hyphens — the dev seed's own is `30-71234567-9` — and an `inputMode` of
   * `numeric` gives iOS a keypad with no hyphen key, so the value cannot be typed on a phone.
   */
  it('leaves the tax id a plain text field', () => {
    const view = renderSignup();

    expect(fieldOf(view, 'taxId')?.getAttribute('inputmode')).toBeNull();
  });

  /*
   * The gate is `trigger` over this step's fields. Without it the caller reaches the last step
   * with a blank corralón name and the failure only surfaces at the end, three steps from the
   * field that caused it.
   */
  it('refuses to advance past a step whose required field is blank', async () => {
    const view = renderSignup();

    fireEvent.submit(view.form);

    await waitFor(() => expect(view.getByText(copy.accountName.required)).toBeTruthy());
    expect(fieldOf(view, 'branchName')).toBeNull();
    expect(signup).not.toHaveBeenCalled();
  });

  /*
   * Advancing goes through `handleSubmit`, not `trigger`: react-hook-form only re-checks a field on
   * change once the form has been submitted, so a step gated with `trigger` alone leaves its
   * required message standing while the caller types the value that answers it.
   */
  it('clears a step message as soon as the field is filled', async () => {
    const view = renderSignup();

    fireEvent.submit(view.form);
    await waitFor(() => expect(view.getByText(copy.accountName.required)).toBeTruthy());

    fill(view, ACCOUNT);
    await waitFor(() =>
      expect(isMessageShown(view.container, copy.accountName.required)).toBe(false),
    );
  });

  /*
   * Advancing marks the whole form submitted, so without a per-step flag the last step would start
   * reporting errors on the first character typed into a field the caller has never submitted.
   */
  it('stays quiet while a step nobody has submitted is being filled in', async () => {
    const view = renderSignup();

    fill(view, ACCOUNT);
    fireEvent.submit(view.form);
    await waitForStep(view, 'branchName');
    fill(view, BRANCH);
    fireEvent.submit(view.form);
    await waitForStep(view, 'adminPassword');

    // Halfway through typing a password: invalid, but not yet submitted on this step.
    fill(view, { adminPassword: 'abc' });
    await new Promise((resolve) => setTimeout(resolve, 0));

    const messages = [...view.container.querySelectorAll('[data-slot="form-message"] p')]
      .map((p) => p.textContent)
      .filter(Boolean);
    expect(messages).toEqual([]);
  });

  it('advances once the step validates, and keeps what was typed on the way back', async () => {
    const view = renderSignup();

    fill(view, ACCOUNT);
    fireEvent.submit(view.form);
    await waitForStep(view, 'branchName');

    fireEvent.click(backButton(view));
    await waitForStep(view, 'accountName');
    expect(fieldOf(view, 'accountName')?.value).toBe(ACCOUNT.accountName);
  });

  /*
   * The step's fields, its button and its stepper have to change in the same commit. Holding the
   * outgoing fields through an exit animation left the screen showing one step's fields under the
   * next step's button, and a second activation in that window submitted the whole form from a
   * step nobody had filled in — three errors waiting on arrival.
   */
  it('never shows a step whose fields disagree with the button', async () => {
    const view = renderSignup();

    fill(view, ACCOUNT);
    fireEvent.submit(view.form);
    await waitForStep(view, 'branchName');
    fill(view, BRANCH);
    fireEvent.submit(view.form);

    // The moment the button becomes the one that creates the account, the fields it submits have
    // to be the ones on screen. Held through an exit animation they were not.
    await waitFor(() => expect(submitButton(view).textContent).toContain(copy.submit));
    expect(fieldOf(view, 'adminEmail')).not.toBeNull();
    expect(fieldOf(view, 'branchName')).toBeNull();
  });

  // Unmounting the step the caller was on drops focus to the body, so tabbing would restart from
  // the top of the page on every step.
  it('moves focus into the step it just revealed', async () => {
    const view = renderSignup();

    fill(view, ACCOUNT);
    fireEvent.submit(view.form);

    await waitFor(() => expect(document.activeElement).toBe(fieldOf(view, 'branchName')));
  });

  // One request creates the account, its branch and its administrator, so the submit carries
  // every step's values however long ago they were typed.
  it('submits all three steps in one call', async () => {
    vi.mocked(signup).mockResolvedValue({ redirectTo: '/verify-email' });
    const view = renderSignup();

    await reachAdminStep(view);
    fill(view, ADMIN);
    fireEvent.submit(view.form);

    await waitFor(() => expect(signup).toHaveBeenCalledOnce());
    expect(signup).toHaveBeenCalledWith({
      ...ACCOUNT,
      ...BRANCH,
      ...ADMIN,
      legalName: '',
      taxId: '',
      branchAddress: '',
    });
  });
});

describe('SignupForm submit button', () => {
  /*
   * `pending` is optional on PendingButton and defaults to false, so a form that passes
   * `pendingLabel` and forgets to wire `pending` type-checks cleanly and silently never shows
   * the state.
   */
  it('goes busy and disables itself while the registration is in flight', async () => {
    let release = () => {};
    vi.mocked(signup).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ redirectTo: '/verify-email' }))),
    );

    const view = renderSignup();
    await reachAdminStep(view);
    fill(view, ADMIN);
    fireEvent.submit(view.form);

    await waitFor(() => expect(submitButton(view).getAttribute('aria-busy')).toBe('true'));
    expect(submitButton(view).disabled).toBe(true);
    expect(submitButton(view).textContent).toContain(copy.submitting);

    release();
    await waitFor(() => expect(signup).toHaveBeenCalledOnce());
  });

  /*
   * The disabled button stops a second click but not a second submit — Enter still reaches the
   * form — and this is the one request in the app that would open a second account.
   */
  it('creates one account when the form is submitted twice', async () => {
    let release = () => {};
    vi.mocked(signup).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ redirectTo: '/verify-email' }))),
    );

    const view = renderSignup();
    await reachAdminStep(view);
    fill(view, ADMIN);
    fireEvent.submit(view.form);
    await waitFor(() => expect(submitButton(view).getAttribute('aria-busy')).toBe('true'));

    fireEvent.submit(view.form);

    /*
     * Flushed rather than waited for. `handleSubmit` calls the action a microtask after the
     * submit, so a `waitFor` on the call count passes on its first poll — before the second
     * call would have landed — and reports one call whether or not the guard is there.
     */
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(signup).toHaveBeenCalledOnce();

    release();
  });
});

describe('SignupForm rejections', () => {
  /*
   * The trap this screen exists to get right. Nothing ties the wizard's step to the form's
   * state, so a message set on a field that is not on screen reads as a button that did
   * nothing — and stepping back while the request is in flight is enough to be somewhere else
   * when the answer lands.
   */
  it('pulls the wizard back to the step owning a refused field', async () => {
    let release = () => {};
    vi.mocked(signup).mockImplementation(
      () =>
        new Promise((resolve) => {
          release = () => resolve({ error: 'EMAIL_TAKEN', field: 'adminEmail' });
        }),
    );

    const view = renderSignup();
    await reachAdminStep(view);
    fill(view, ADMIN);
    fireEvent.submit(view.form);

    // Away from the step the answer belongs to, while the request is still open.
    await waitFor(() => expect(submitButton(view).getAttribute('aria-busy')).toBe('true'));
    fireEvent.click(backButton(view));
    await waitForStep(view, 'branchName');

    release();

    await waitForStep(view, 'adminEmail');
    expect(view.getByText(messages.errors.EMAIL_TAKEN)).toBeTruthy();
    expect(fieldOf(view, 'adminEmail')?.getAttribute('aria-invalid')).toBe('true');
    // On the refused field, not merely on the step: it is the one the caller has to change.
    await waitFor(() => expect(document.activeElement).toBe(fieldOf(view, 'adminEmail')));
  });

  it('reports a failure that belongs to no field on the form itself', async () => {
    vi.mocked(signup).mockResolvedValue({ error: 'UNREACHABLE' });

    const view = renderSignup();
    await reachAdminStep(view);
    fill(view, ADMIN);
    fireEvent.submit(view.form);

    await waitFor(() => expect(view.getByText(messages.errors.UNREACHABLE)).toBeTruthy());
  });
});
