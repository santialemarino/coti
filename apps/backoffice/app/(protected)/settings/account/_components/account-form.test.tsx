import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { AccountForm } from '@/app/(protected)/settings/account/_components/account-form';
import type { Account } from '@/lib/api/account';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/account/actions', () => ({ updateAccount: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { updateAccount } = await import('@/app/(protected)/settings/account/actions');
const { toast } = await import('sonner');

const copy = messages.account;

const ACCOUNT: Account = {
  id: 'a1',
  name: 'Corralón San Martín',
  legalName: 'Corralón San Martín S.R.L.',
  taxId: '30-71234567-9',
  brandLogoUrl: 'https://tucorralon.com/logo.png',
  brandColor: '#C2410C',
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm(account: Account = ACCOUNT) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <AccountForm account={account} />
    </NextIntlClientProvider>,
  );
}

function field(view: RenderResult, name: string): HTMLInputElement {
  const input = view.baseElement.querySelector(`input[name="${name}"]`);
  if (!(input instanceof HTMLInputElement)) throw new Error(`no ${name} field on screen`);
  return input;
}

// Clicking a submit button does not reach a React form in jsdom, which implements no requestSubmit.
function submit(view: RenderResult) {
  const form = view.baseElement.querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form on screen');
  fireEvent.submit(form);
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe('AccountForm', () => {
  it('opens on the values the account already has', () => {
    const view = renderForm();

    expect(field(view, 'name').value).toBe(ACCOUNT.name);
    expect(field(view, 'legalName').value).toBe(ACCOUNT.legalName);
    expect(field(view, 'taxId').value).toBe(ACCOUNT.taxId);
    expect(view.getByRole('img', { name: messages.common.logoUpload.previewAlt })).toBeTruthy();
    expect(field(view, 'brandColor').value).toBe('C2410C');
  });

  // Null is "never set"; a text input can only hold a string, and the empty one is what the action
  // then omits from the body.
  it('shows an unset optional field as empty rather than as null', () => {
    const view = renderForm({ ...ACCOUNT, legalName: null, taxId: null, brandColor: null });

    expect(field(view, 'legalName').value).toBe('');
    expect(field(view, 'taxId').value).toBe('');
    expect(field(view, 'brandColor').value).toBe('');
  });

  /*
   * Not cosmetic: react-hook-form would hold the null, the schema refuses a null string, and the
   * form would fail its own validation on a field nobody touched — so an account with no legal name
   * could never save anything again.
   */
  it('submits an unset optional field as the empty string the API omits', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm({
      ...ACCOUNT,
      legalName: null,
      taxId: null,
      brandLogoUrl: null,
      brandColor: null,
    });

    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(
      {
        name: ACCOUNT.name,
        legalName: '',
        taxId: '',
        brandLogoUrl: '',
        brandColor: '',
      },
      undefined,
    );
  });

  it('sends what is on screen and says it saved', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();

    fireEvent.change(field(view, 'name'), { target: { value: 'Corralón San Martín Sur' } });
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(
      {
        name: 'Corralón San Martín Sur',
        legalName: ACCOUNT.legalName,
        taxId: ACCOUNT.taxId,
        brandLogoUrl: ACCOUNT.brandLogoUrl,
        brandColor: 'C2410C',
      },
      undefined,
    );
    // A confirmation of something just done is transient, so it is a toast.
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.saved));
  });

  it('refuses to submit without a name, and says so on the field', async () => {
    const view = renderForm();

    fireEvent.change(field(view, 'name'), { target: { value: '   ' } });
    submit(view);

    await waitFor(() => expect(view.getByText(copy.name.required)).toBeTruthy());
    expect(updateAccount).not.toHaveBeenCalled();
  });

  /*
   * The rejection belongs to the form: the codes the API answers with are all values this schema
   * already refuses, so there is no field to point at. A code this flow words itself, so a screen
   * bound to the wrong namespace fails here instead of falling back to a sentence that reads fine.
   */
  it('puts a refused save on the form, not on a field', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ error: 'NOT_FOUND' });
    const view = renderForm();

    submit(view);

    await waitFor(() => expect(view.getByText(copy.errors.NOT_FOUND)).toBeTruthy());
    expect(copy.errors.NOT_FOUND).not.toBe(messages.errors.NOT_FOUND);
    expect(toast.success).not.toHaveBeenCalled();
  });

  /*
   * `PendingButton`'s `pending` prop is optional and defaults to false, so a form that passes the
   * label and forgets to wire the state type-checks cleanly and silently never shows it.
   */
  it('shows the pending state on the submit button', async () => {
    let release = () => {};
    vi.mocked(updateAccount).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ ok: true }))),
    );
    const view = renderForm();
    // Held by node, not re-queried by name: both labels are on it while they crossfade.
    const button = view.baseElement.querySelector('button[type="submit"]');
    if (!(button instanceof HTMLButtonElement)) throw new Error('no submit button on screen');

    submit(view);

    await waitFor(() => expect(button.getAttribute('aria-busy')).toBe('true'));
    expect(button).toHaveProperty('disabled', true);
    expect(button.textContent).toContain(copy.submitting);

    release();
    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
  });
});

describe('AccountForm brand controls', () => {
  it('keeps the hash outside the editable colour value', () => {
    const view = renderForm();

    expect(field(view, 'brandColor').value).toBe('C2410C');
    expect(view.getByText('#')).toBeTruthy();
  });

  it('copies the native colour picker value into the text field without its hash', () => {
    const view = renderForm();
    const picker = view.getByLabelText(copy.brandColor.pickerLabel);

    fireEvent.input(picker, { target: { value: '#12abef' } });

    expect(field(view, 'brandColor').value).toBe('12ABEF');
  });

  it('hands the selected logo file to the save action', async () => {
    vi.stubGlobal(
      'URL',
      class extends URL {
        static createObjectURL = vi.fn(() => 'blob:logo');
        static revokeObjectURL = vi.fn();
      },
    );
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();
    const logo = new File(['logo'], 'logo.png', { type: 'image/png' });
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');

    fireEvent.change(input!, { target: { files: [logo] } });
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(expect.any(Object), logo);
  });

  it('clears the stored logo when it is removed', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();

    fireEvent.click(view.getByRole('button', { name: messages.common.logoUpload.remove }));
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(expect.any(Object), null);
  });
});
