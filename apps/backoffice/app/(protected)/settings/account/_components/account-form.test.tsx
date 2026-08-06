import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

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

function swatch(view: RenderResult): HTMLElement {
  const found = view.baseElement.querySelector('[data-slot="brand-swatch"]');
  if (!(found instanceof HTMLElement)) throw new Error('no swatch on screen');
  return found;
}

beforeEach(() => vi.clearAllMocks());

describe('AccountForm', () => {
  it('opens on the values the account already has', () => {
    const view = renderForm();

    expect(field(view, 'name').value).toBe(ACCOUNT.name);
    expect(field(view, 'legalName').value).toBe(ACCOUNT.legalName);
    expect(field(view, 'taxId').value).toBe(ACCOUNT.taxId);
    expect(field(view, 'brandLogoUrl').value).toBe(ACCOUNT.brandLogoUrl);
    expect(field(view, 'brandColor').value).toBe(ACCOUNT.brandColor);
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
    expect(updateAccount).toHaveBeenCalledWith({
      name: ACCOUNT.name,
      legalName: '',
      taxId: '',
      brandLogoUrl: '',
      brandColor: '',
    });
  });

  it('sends what is on screen and says it saved', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();

    fireEvent.change(field(view, 'name'), { target: { value: 'Corralón San Martín Sur' } });
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith({
      name: 'Corralón San Martín Sur',
      legalName: ACCOUNT.legalName,
      taxId: ACCOUNT.taxId,
      brandLogoUrl: ACCOUNT.brandLogoUrl,
      brandColor: ACCOUNT.brandColor,
    });
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

/*
 * The brand preview is the only feedback on a colour before saving, and it is deliberately driven
 * by the same shape the API accepts: a swatch that goes blank is what says the value is malformed.
 */
describe('AccountForm brand preview', () => {
  it('paints the swatch with the colour on screen', () => {
    const view = renderForm();

    expect(swatch(view).style.backgroundColor).toBe('rgb(194, 65, 12)');
  });

  // Five digits, not four: `#C241` is a valid four-digit hex with an alpha channel, which the API
  // accepts and this mirror therefore must too.
  it('leaves the swatch unpainted while the colour is malformed', async () => {
    const view = renderForm();

    fireEvent.change(field(view, 'brandColor'), { target: { value: '#C2410' } });

    await waitFor(() => expect(swatch(view).style.backgroundColor).toBe(''));
  });

  it('paints it again as soon as the colour is one the API would store', async () => {
    const view = renderForm({ ...ACCOUNT, brandColor: null });

    fireEvent.change(field(view, 'brandColor'), { target: { value: '#0F0' } });

    await waitFor(() => expect(swatch(view).style.backgroundColor).toBe('rgb(0, 255, 0)'));
  });

  // Opened rather than rendered: the backoffice does not load an address someone pasted into a
  // field, and one click is enough to confirm it is the right image.
  it('offers the logo as a link once the address is a complete one', async () => {
    const view = renderForm({ ...ACCOUNT, brandLogoUrl: null });
    expect(view.queryByRole('link', { name: copy.brandLogoUrl.open })).toBeNull();

    fireEvent.change(field(view, 'brandLogoUrl'), {
      target: { value: 'https://tucorralon.com/logo.png' },
    });

    const link = await waitFor(() => view.getByRole('link', { name: copy.brandLogoUrl.open }));
    expect(link.getAttribute('href')).toBe('https://tucorralon.com/logo.png');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noreferrer');
  });

  it('offers no link while the address is incomplete', async () => {
    const view = renderForm();

    fireEvent.change(field(view, 'brandLogoUrl'), { target: { value: 'tucorralon.com/logo.png' } });

    await waitFor(() =>
      expect(view.queryByRole('link', { name: copy.brandLogoUrl.open })).toBeNull(),
    );
  });
});
