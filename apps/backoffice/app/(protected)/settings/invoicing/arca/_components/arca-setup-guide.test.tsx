import { act, fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ARCASetupGuide } from '@/app/(protected)/settings/invoicing/arca/_components/arca-setup-guide';
import { ROUTES } from '@/config/routes';
import type { ARCASetup } from '@/lib/api/arca-setup';
import type { InvoicingBranch } from '@/lib/api/invoicing';
import type { ARCAConnectionState } from '@/lib/utils/arca-connection';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('next/navigation')>()),
  useRouter: () => ({ refresh }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/settings/invoicing/arca/actions', () => ({
  createARCASetup: vi.fn(),
  uploadARCACertificate: vi.fn(),
  disconnectARCA: vi.fn(),
}));
vi.mock('@/app/(protected)/settings/invoicing/actions', () => ({ updatePointsOfSale: vi.fn() }));

const { toast } = await import('sonner');
const { createARCASetup, uploadARCACertificate, disconnectARCA } =
  await import('@/app/(protected)/settings/invoicing/arca/actions');
const { updatePointsOfSale } = await import('@/app/(protected)/settings/invoicing/actions');

const copy = messages.arca;
const connection = messages.invoicing.settings.connection;
const CERTIFICATE = '-----BEGIN CERTIFICATE-----\nMIIDazCCAlOgAwIBAgIU\n-----END CERTIFICATE-----';
const EMPTY: ARCASetup = {
  enabled: true,
  taxId: '',
  csr: '',
  hasCertificate: false,
  certificate: '',
  certificateExpiresAt: null,
};
const REQUESTED: ARCASetup = {
  ...EMPTY,
  taxId: '30712345671',
  csr: '-----BEGIN CERTIFICATE REQUEST-----\nMIIC\n-----END CERTIFICATE REQUEST-----',
};
const INSTALLED: ARCASetup = {
  ...REQUESTED,
  hasCertificate: true,
  certificate: CERTIFICATE,
  certificateExpiresAt: '2028-10-13T00:00:00Z',
};
const REQUEST_FILE_TEXT =
  '-----BEGIN CERTIFICATE REQUEST-----\nMIIC\n-----END CERTIFICATE REQUEST-----';
const BRANCHES: InvoicingBranch[] = [
  { branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 1 },
  { branchId: 'b', name: 'Villa Bosch', isActive: true, pointOfSale: null },
];
const READY: InvoicingBranch[] = [
  { branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 1 },
  { branchId: 'b', name: 'Villa Bosch', isActive: true, pointOfSale: 2 },
];

function show(
  setup: ARCASetup,
  state: ARCAConnectionState,
  { accountTaxId = '30-71234567-1', branches = BRANCHES } = {} as {
    accountTaxId?: string | null;
    branches?: InvoicingBranch[];
  },
) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ARCASetupGuide setup={setup} accountTaxId={accountTaxId} branches={branches} state={state} />
    </NextIntlClientProvider>,
  );
}

function stepState(view: ReturnType<typeof show>, title: string) {
  return view.getByRole('heading', { name: new RegExp(`^${title}`) }).closest('li')?.dataset.status;
}

function statuses(view: ReturnType<typeof show>) {
  return Array.from(view.container.querySelectorAll('[data-slot=step-guide] > li')).map(
    (li) => (li as HTMLElement).dataset.status,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe('ARCASetupGuide states', () => {
  it('starts on WSASS and the request, with the account CUIT shown read-only', () => {
    const view = show(EMPTY, 'notConnected');

    expect(stepState(view, copy.steps.wsass.title)).toBe('current');
    expect(stepState(view, copy.steps.generate.title)).toBe('current');
    expect(stepState(view, copy.steps.upload.title)).toBe('pending');
    expect(view.getByText(copy.steps.generate.cuit).nextElementSibling?.textContent).toContain(
      '30-71234567-1',
    );
    expect(view.queryByRole('textbox', { name: /CUIT/ })).toBeNull();
    expect(
      view.getByRole('link', { name: copy.steps.generate.editCuit }).getAttribute('href'),
    ).toBe(ROUTES.accountSettings);
  });

  it('titles the steps one level under the page heading', () => {
    const view = show(EMPTY, 'notConnected');
    expect(view.getAllByRole('heading', { level: 2 })).toHaveLength(7);
  });

  it.each([null, '30-71234567-9'])('blocks the request without a valid CUIT (%s)', (cuit) => {
    const view = show(EMPTY, 'notConnected', { accountTaxId: cuit });

    expect(view.getByText(messages.common.setup.ACCOUNT_TAX_ID.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.steps.generate.submit })).toBeNull();
  });

  it('generates the request without sending a CUIT', async () => {
    vi.mocked(createARCASetup).mockResolvedValue({ ok: true });
    const view = show(EMPTY, 'notConnected');

    fireEvent.click(view.getByRole('button', { name: copy.steps.generate.submit }));

    await waitFor(() => expect(createARCASetup).toHaveBeenCalledWith());
    expect(refresh).toHaveBeenCalled();
  });

  it('moves to the steps done in ARCA once the request exists, the request ready to copy', () => {
    const view = show(REQUESTED, 'inProgress');

    expect(statuses(view)).toEqual([
      'done',
      'done',
      'current',
      'current',
      'current',
      'current',
      'pending',
    ]);
    const request = view.getByText(copy.steps.copy.label).closest('figure')!;
    expect(request.querySelector('figcaption')?.textContent).toBe(copy.steps.copy.label);
    expect(request.textContent).toContain(REQUESTED.csr);
  });

  // The full way to WSASS, with ARCA's exact label, is given once; later steps only go back to it.
  it('explains how to find WSASS once and sends the later steps back to it', () => {
    const view = show(REQUESTED, 'inProgress');
    const text = view.container.textContent ?? '';
    expect(text.split('Autogestión Certificados Homologación').length - 1).toBe(1);
    expect(
      view.getByText(copy.steps.wsass.check.replace('{lookup}', copy.wsassLookup)),
    ).toBeTruthy();
    expect(view.getByText(copy.steps.certificate.open)).toBeTruthy();
    expect(text).toContain('Volvé a «WSASS» y elegí «Crear autorización a servicio»');
  });

  it('shows the certificate’s real delimiters and describes the middle, with one instruction', () => {
    const view = show(REQUESTED, 'inProgress');
    const example = view.getByText(copy.pem.instruction).closest('figure')!;
    expect(within(example).getByText('-----BEGIN CERTIFICATE-----')).toBeTruthy();
    expect(within(example).getByText('-----END CERTIFICATE-----')).toBeTruthy();
    expect(within(example).getByText(copy.pem.middle).tagName).not.toBe('CODE');
    expect(view.container.textContent).not.toContain('MIIDaz');
    expect(JSON.stringify(copy)).not.toMatch(/BEGIN|END CERTIFICATE/);
  });

  it('agrees with the status card while a point of sale is missing', () => {
    const view = show(INSTALLED, 'pointsMissing');

    expect(stepState(view, copy.steps.pointsOfSale.title)).toBe('current');
    expect(view.getByText(connection.states.pointsMissing)).toBeTruthy();
    expect(view.getByText('1 de 2 sucursales abiertas tienen punto de venta.')).toBeTruthy();
    expect(view.container.querySelector('#points-of-sale')).toBeTruthy();
  });

  it('finishes every step once every open branch has a point of sale', () => {
    const view = show(INSTALLED, 'connected', { branches: READY });
    expect(statuses(view).every((status) => status === 'done')).toBe(true);
    expect(view.queryByRole('alert')).toBeNull();
  });

  it.each([
    ['expiringSoon', /El certificado vence el/],
    ['expired', /El certificado venció el/],
  ] as const)('calls out the certificate when it is %s', (state, upload) => {
    const view = show(INSTALLED, state, { branches: READY });
    expect(view.getByText(connection.states[state])).toBeTruthy();
    if (state === 'expired') expect(view.getByText(upload)).toBeTruthy();
  });

  it('explains a server that cannot connect, with no steps to take', () => {
    const view = show({ ...EMPTY, enabled: false }, 'disabled');
    expect(view.getByText(copy.disabled)).toBeTruthy();
    expect(view.getByText(copy.disabled).closest('[role="status"]')).toBeNull();
    expect(view.queryByRole('list')).toBeNull();
  });
});

describe('ARCASetupGuide copy and download', () => {
  it('copies the request and says so', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
    const view = show(REQUESTED, 'inProgress');

    fireEvent.click(view.getByRole('button', { name: copy.steps.copy.copy }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(REQUESTED.csr));
    expect((await view.findAllByText(copy.steps.copy.copied)).length).toBeGreaterThan(0);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it('points to the download when the clipboard refuses', async () => {
    vi.stubGlobal('navigator', {
      ...navigator,
      clipboard: { writeText: vi.fn(async () => Promise.reject(new Error('denied'))) },
    });
    const view = show(REQUESTED, 'inProgress');

    fireEvent.click(view.getByRole('button', { name: copy.steps.copy.copy }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.steps.copy.copyError));
  });

  it.each([
    [
      'request',
      REQUESTED,
      'inProgress',
      copy.steps.copy.download,
      'coti-30712345671.csr',
      REQUESTED.csr,
    ],
    [
      'certificate',
      INSTALLED,
      'connected',
      copy.steps.upload.download,
      'coti-30712345671.pem',
      CERTIFICATE,
    ],
  ] as const)('downloads the %s as a file', async (_, setup, state, label, fileName, content) => {
    const blobs: Blob[] = [];
    const createObjectURL = vi.fn((blob: Blob) => {
      blobs.push(blob);
      return 'blob:download';
    });
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() }));
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      expect(this.download).toBe(fileName);
    });
    const view = show(setup, state, { branches: READY });

    fireEvent.click(view.getByRole('button', { name: label }));

    expect(click).toHaveBeenCalledOnce();
    expect(await blobs[0]!.text()).toBe(content);
    click.mockRestore();
  });
});

describe('ARCASetupGuide certificate', () => {
  it('sends a pasted certificate trimmed', async () => {
    vi.mocked(uploadARCACertificate).mockResolvedValue({ ok: true });
    const view = show(REQUESTED, 'inProgress');

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: `\n  ${CERTIFICATE}  \n` },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(uploadARCACertificate).toHaveBeenCalledWith(CERTIFICATE));
    expect(toast.success).toHaveBeenCalledWith(copy.steps.upload.saved);
  });

  it('asks for the paste in the field, not with an example inside it', () => {
    const view = show(REQUESTED, 'inProgress');
    const field = view.getByLabelText(copy.steps.upload.label, { exact: false });
    expect(field.getAttribute('placeholder')).toBe(copy.steps.upload.placeholder);
  });

  it('refuses text that is not one certificate block, without calling the API', async () => {
    const view = show(REQUESTED, 'inProgress');

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: 'MIIDazCCAlOgAwIBAgIU' },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(view.getByText(copy.steps.upload.invalid)).toBeTruthy());
    expect(uploadARCACertificate).not.toHaveBeenCalled();
  });

  function certificateFile() {
    const file = new File([CERTIFICATE], 'certificado.pem');
    Object.defineProperty(file, 'text', { value: async () => CERTIFICATE });
    return file;
  }

  it('reads a chosen file into the same field and says which one', async () => {
    const view = show(REQUESTED, 'inProgress');
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');

    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.chooseFile }));
    fireEvent.change(input!, { target: { files: [certificateFile()] } });

    await waitFor(() =>
      expect(
        (view.getByLabelText(copy.steps.upload.label, { exact: false }) as HTMLTextAreaElement)
          .value,
      ).toBe(CERTIFICATE),
    );
    const note = 'Leímos el certificado de certificado.pem. Revisalo y tocá «Guardar certificado».';
    expect(view.getAllByText(note)).toHaveLength(2);
    expect(view.container.querySelector('[data-slot=certificate-loaded]')?.textContent).toBe(note);
    expect(view.queryByText(/Arrastrá/)).toBeNull();

    // Editing the text drops the note: what is in the field is no longer what the file held.
    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: `${CERTIFICATE}x` },
    });
    expect(view.container.querySelector('[data-slot=certificate-loaded]')?.textContent).toBe('');
  });

  it.each([
    ['the request generated in Coti', REQUEST_FILE_TEXT, 'fileRequest'],
    ['anything else', 'hola', 'fileInvalid'],
  ] as const)('refuses a file holding %s, leaving the field as it was', async (_, text, key) => {
    const view = show(REQUESTED, 'inProgress');
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');
    const file = new File([text], 'archivo.txt');
    Object.defineProperty(file, 'text', { value: async () => text });

    fireEvent.change(input!, { target: { files: [file] } });

    const field = view.getByLabelText(copy.steps.upload.label, { exact: false });
    await waitFor(() => expect(view.getByText(copy.steps.upload[key])).toBeTruthy());
    expect((field as HTMLTextAreaElement).value).toBe('');
    expect(view.container.querySelector('[data-slot=certificate-loaded]')?.textContent).toBe('');
    expect(input?.getAttribute('accept')).toBe('.crt,.pem,.cer,.csr,.txt');
  });

  it('tells a pasted request apart from the certificate', async () => {
    const view = show(REQUESTED, 'inProgress');

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: REQUEST_FILE_TEXT },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(view.getByText(copy.steps.upload.pastedRequest)).toBeTruthy());
    expect(uploadARCACertificate).not.toHaveBeenCalled();
  });

  it('fills the field from a file dropped on it, highlighted while it is dragged over', async () => {
    const view = show(REQUESTED, 'inProgress');
    const field = view.getByLabelText(copy.steps.upload.label, { exact: false });
    const dataTransfer = { types: ['Files'], files: [certificateFile()] };

    fireEvent.dragOver(field, { dataTransfer });
    expect(field.dataset.dragging).toBe('true');
    await act(async () => {
      fireEvent.drop(field, { dataTransfer });
    });

    expect(field.dataset.dragging).toBeUndefined();
    await waitFor(() => expect((field as HTMLTextAreaElement).value).toBe(CERTIFICATE));
  });

  it('focuses the certificate field when the API refuses it', async () => {
    vi.mocked(uploadARCACertificate).mockResolvedValue({ error: 'INVALID_INPUT' });
    const view = show(REQUESTED, 'inProgress');
    const field = view.getByLabelText(copy.steps.upload.label, { exact: false });

    fireEvent.change(field, { target: { value: CERTIFICATE } });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(document.activeElement).toBe(field));
  });

  it('puts the API’s refusal on the certificate field', async () => {
    vi.mocked(uploadARCACertificate).mockResolvedValue({ error: 'INVALID_INPUT' });
    const view = show(REQUESTED, 'inProgress');

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: CERTIFICATE },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(view.getByText(copy.errors.INVALID_INPUT)).toBeTruthy());
  });
});

describe('ARCASetupGuide points of sale', () => {
  it('says what comes after the certificate, with no table yet', () => {
    const view = show(REQUESTED, 'inProgress');
    expect(view.getByText(copy.steps.pointsOfSale.pending)).toBeTruthy();
    expect(view.queryByLabelText('Punto de venta de Morón')).toBeNull();
  });

  it('saves only the rows that changed, from inside the step', async () => {
    vi.mocked(updatePointsOfSale).mockResolvedValue({
      settings: { branches: READY } as never,
    });
    const view = show(INSTALLED, 'pointsMissing');

    fireEvent.change(view.getByLabelText('Punto de venta de Villa Bosch'), {
      target: { value: '2' },
    });
    fireEvent.click(
      view.getByRole('button', { name: messages.invoicing.settings.pointsOfSale.save }),
    );

    await waitFor(() =>
      expect(updatePointsOfSale).toHaveBeenCalledWith([{ branchId: 'b', pointOfSale: '2' }]),
    );
    expect(refresh).toHaveBeenCalled();
  });

  it('refuses one number for two branches before saving', async () => {
    const view = show(INSTALLED, 'pointsMissing');

    fireEvent.change(view.getByLabelText('Punto de venta de Villa Bosch'), {
      target: { value: '1' },
    });
    fireEvent.click(
      view.getByRole('button', { name: messages.invoicing.settings.pointsOfSale.save }),
    );

    await waitFor(() =>
      expect(view.getByText(messages.invoicing.settings.pointOfSale.duplicate)).toBeTruthy(),
    );
    expect(updatePointsOfSale).not.toHaveBeenCalled();
  });
});

describe('ARCASetupGuide removal', () => {
  it('starts over while the connection is in progress', async () => {
    vi.mocked(disconnectARCA).mockResolvedValue({ ok: true });
    const view = show(REQUESTED, 'inProgress');

    expect(view.getByText(copy.remove.restart.hint)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.remove.disconnect.action })).toBeNull();
    fireEvent.click(view.getByRole('button', { name: copy.remove.restart.action }));
    const dialog = await view.findByRole('dialog');
    expect(within(dialog).getByText(/30-71234567-1/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: copy.remove.restart.action }));

    await waitFor(() => expect(disconnectARCA).toHaveBeenCalledOnce());
    expect(toast.success).toHaveBeenCalledWith(copy.remove.restart.done);
  });

  it('asks first, then disconnects once connected', async () => {
    vi.mocked(disconnectARCA).mockResolvedValue({ ok: true });
    const view = show(INSTALLED, 'connected', { branches: READY });

    fireEvent.click(view.getByRole('button', { name: copy.remove.disconnect.action }));
    const dialog = await view.findByRole('dialog');
    expect(disconnectARCA).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: copy.remove.disconnect.action }));

    await waitFor(() => expect(disconnectARCA).toHaveBeenCalledOnce());
    expect(toast.success).toHaveBeenCalledWith(copy.remove.disconnect.done);
  });

  it('keeps the connection while an invoice waits for ARCA, and says why', async () => {
    vi.mocked(disconnectARCA).mockResolvedValue({ error: 'INVOICE_PENDING' });
    const view = show(INSTALLED, 'connected', { branches: READY });

    fireEvent.click(view.getByRole('button', { name: copy.remove.disconnect.action }));
    const dialog = await view.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: copy.remove.disconnect.action }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.INVOICE_PENDING));
    expect(refresh).not.toHaveBeenCalled();
  });

  it('offers nothing to remove before there is a request', () => {
    const view = show(EMPTY, 'notConnected');
    expect(view.queryByRole('button', { name: copy.remove.restart.action })).toBeNull();
  });
});
