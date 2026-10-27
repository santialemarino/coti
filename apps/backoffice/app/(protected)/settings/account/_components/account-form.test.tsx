import { fireEvent, render, waitFor, within, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { isMessageShown } from '@repo/vitest-config/form-messages';
import { AccountForm } from '@/app/(protected)/settings/account/_components/account-form';
import type { Account } from '@/lib/api/account';
import { BRAND_COLOR_PRESETS } from '@/lib/constants/brand';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/account/actions', () => ({ updateAccount: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { updateAccount } = await import('@/app/(protected)/settings/account/actions');
const { toast } = await import('sonner');

const copy = messages.account;

vi.mock('@/components/logo-crop-dialog', () => ({
  LogoCropDialog: ({
    open,
    file,
    onConfirm,
  }: {
    open: boolean;
    file: File;
    onConfirm: (file: File) => void;
  }) =>
    open ? (
      <button type="button" onClick={() => onConfirm(file)}>
        Use crop
      </button>
    ) : null,
}));
const ACCOUNT: Account = {
  id: 'a1',
  name: 'Corralón San Martín',
  legalName: 'Corralón San Martín S.R.L.',
  taxId: '30-71234567-1',
  brandLogoUrl: 'https://tucorralon.com/logo.png',
  brandColor: '#C2410C',
  preferredLowerPriceStrategy: 'FIND_CHEAPER_ALTERNATIVES',
  invoicingEnabled: false,
  hasInvoices: false,
  invoiceOnPayment: false,
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm(
  account: Account = ACCOUNT,
  arcaConnection: 'inProgress' | 'connected' | null = null,
) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <AccountForm account={account} arcaConnection={arcaConnection} />
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
        lowerPriceStrategy: 'FIND_CHEAPER_ALTERNATIVES',
      },
      undefined,
      false,
    );
  });

  // The stored choice is the one selected, and picking the other is what the save sends.
  it('shows the stored lower-price strategy and sends the one picked', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();
    const cheaper = view.getByRole('radio', {
      name: copy.lowerPrice.FIND_CHEAPER_ALTERNATIVES.label,
    });
    const adapt = view.getByRole('radio', { name: copy.lowerPrice.ADAPT_TO_REACH_DISCOUNT.label });
    expect(cheaper.getAttribute('aria-checked')).toBe('true');

    fireEvent.click(adapt);
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(vi.mocked(updateAccount).mock.calls[0]?.[0].lowerPriceStrategy).toBe(
      'ADAPT_TO_REACH_DISCOUNT',
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
        taxId: '30712345671',
        brandLogoUrl: ACCOUNT.brandLogoUrl,
        brandColor: 'C2410C',
        lowerPriceStrategy: 'FIND_CHEAPER_ALTERNATIVES',
      },
      undefined,
      false,
    );
    // A confirmation of something just done is transient, so it is a toast.
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.saved));
  });

  // The field shows the CUIT as it is written and describes that shape to a screen reader too.
  it('shows the CUIT formatted, with its format as the field description', () => {
    const view = renderForm({ ...ACCOUNT, taxId: '30712345671' });
    const taxId = field(view, 'taxId');

    expect(taxId.value).toBe('30-71234567-1');
    expect(taxId.getAttribute('inputmode')).toBe('numeric');
    const described = taxId.getAttribute('aria-describedby')?.split(' ') ?? [];
    expect(described.map((id) => document.getElementById(id)?.textContent)).toContain(
      copy.taxId.hint,
    );
  });

  it('refuses a CUIT with the wrong check digit, then clears the message once it is fixed', async () => {
    const view = renderForm();

    fireEvent.change(field(view, 'taxId'), { target: { value: '30-71234567-9' } });
    submit(view);

    await waitFor(() => expect(isMessageShown(view.container, copy.taxId.checkDigit)).toBe(true));
    expect(field(view, 'taxId').getAttribute('aria-invalid')).toBe('true');
    expect(updateAccount).not.toHaveBeenCalled();

    fireEvent.change(field(view, 'taxId'), { target: { value: '30-71234567-1' } });

    await waitFor(() => expect(isMessageShown(view.container, copy.taxId.checkDigit)).toBe(false));
  });

  it('refuses a CUIT that is too short to be one', async () => {
    const view = renderForm({ ...ACCOUNT, taxId: null });

    fireEvent.change(field(view, 'taxId'), { target: { value: '3071234' } });
    submit(view);

    await waitFor(() => expect(isMessageShown(view.container, copy.taxId.invalid)).toBe(true));
    expect(updateAccount).not.toHaveBeenCalled();
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

  // A CUIT ARCA is connected with changes only once the caller accepts losing the connection.
  it.each([
    ['inProgress', copy.disconnectARCA.inProgress],
    ['connected', copy.disconnectARCA.connected],
  ] as const)('asks before a CUIT change disconnects ARCA (%s)', async (state, description) => {
    vi.mocked(updateAccount)
      .mockResolvedValueOnce({ error: 'ARCA_CONNECTED' })
      .mockResolvedValueOnce({ ok: true });
    const view = renderForm(ACCOUNT, state);

    submit(view);

    const dialog = await view.findByRole('dialog');
    expect(within(dialog).getByText(description.replace('{old}', ACCOUNT.taxId!))).toBeTruthy();
    expect(updateAccount).toHaveBeenLastCalledWith(expect.anything(), undefined, false);
    fireEvent.click(within(dialog).getByRole('button', { name: copy.disconnectARCA.confirm }));

    await waitFor(() =>
      expect(updateAccount).toHaveBeenLastCalledWith(expect.anything(), undefined, true),
    );
    expect(toast.success).toHaveBeenCalledWith(copy.savedDisconnected);
  });

  it('saves nothing when the change is cancelled', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ error: 'ARCA_CONNECTED' });
    const view = renderForm(ACCOUNT, 'connected');

    submit(view);
    const dialog = await view.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: copy.disconnectARCA.cancel }));

    await waitFor(() => expect(view.queryByRole('dialog')).toBeNull());
    expect(updateAccount).toHaveBeenCalledOnce();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('lands on the CUIT when a confirmed change meets a pending invoice', async () => {
    vi.mocked(updateAccount)
      .mockResolvedValueOnce({ error: 'ARCA_CONNECTED' })
      .mockResolvedValueOnce({ error: 'INVOICE_PENDING' });
    const view = renderForm(ACCOUNT, 'connected');
    // A real save starts from the focused button, which is where the dialog would hand focus back.
    view.container.querySelector<HTMLButtonElement>('button[type="submit"]')!.focus();

    submit(view);
    const dialog = await view.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: copy.disconnectARCA.confirm }));

    await waitFor(() => expect(view.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(field(view, 'taxId')));
  });

  it('puts a CUIT the API refuses on its field', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ error: 'INVALID_TAX_ID' });
    const view = renderForm();

    submit(view);

    await waitFor(() => expect(field(view, 'taxId').getAttribute('aria-invalid')).toBe('true'));
    expect(view.getByText(copy.errors.INVALID_TAX_ID)).toBeTruthy();
  });

  it('points to the invoices that keep the CUIT from changing', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ error: 'INVOICE_PENDING' });
    const view = renderForm();

    submit(view);

    await waitFor(() => expect(field(view, 'taxId').getAttribute('aria-invalid')).toBe('true'));
    expect(view.getByText(copy.invoicePending, { exact: false })).toBeTruthy();
    expect(view.getByRole('link', { name: copy.invoicePendingLink }).getAttribute('href')).toBe(
      '/administration',
    );
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

  // The picker writes into the same field the hex input owns, and the hash stays a prefix.
  it('copies a picked colour into the text field without its hash', async () => {
    const view = renderForm();

    fireEvent.click(view.getByLabelText(copy.brandColor.pickerLabel));
    const preset = await waitFor(() => view.getByLabelText(`#${BRAND_COLOR_PRESETS[4]}`));
    fireEvent.click(preset);

    expect(field(view, 'brandColor').value).toBe(BRAND_COLOR_PRESETS[4]);
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
    fireEvent.click(view.getByRole('button', { name: 'Use crop' }));
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(expect.any(Object), logo, false);
  });

  it('clears the stored logo when it is removed', async () => {
    vi.mocked(updateAccount).mockResolvedValue({ ok: true });
    const view = renderForm();

    fireEvent.click(view.getByRole('button', { name: messages.common.logoUpload.remove }));
    submit(view);

    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(updateAccount).toHaveBeenCalledWith(expect.any(Object), null, false);
  });
});
