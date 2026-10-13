import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ARCASetupGuide } from '@/app/(protected)/settings/invoicing/arca/_components/arca-setup-guide';
import { ROUTES } from '@/config/routes';
import type { ARCASetup } from '@/lib/api/arca-setup';
import type { InvoicingBranch } from '@/lib/api/invoicing';
import messages from '@/translations/es.json';

const refresh = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/settings/invoicing/arca/actions', () => ({
  createARCASetup: vi.fn(),
  uploadARCACertificate: vi.fn(),
  disconnectARCA: vi.fn(),
}));

const { toast } = await import('sonner');
const { createARCASetup, uploadARCACertificate, disconnectARCA } =
  await import('@/app/(protected)/settings/invoicing/arca/actions');

const copy = messages.arca;
const NOW = '2026-10-13T12:00:00.000Z';
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
const BRANCHES: InvoicingBranch[] = [
  { branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 1 },
  { branchId: 'b', name: 'Villa Bosch', isActive: true, pointOfSale: null },
];

function show(setup: ARCASetup, accountTaxId: string | null = '30-71234567-1') {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ARCASetupGuide setup={setup} accountTaxId={accountTaxId} branches={BRANCHES} now={NOW} />
    </NextIntlClientProvider>,
  );
}

function stepState(view: ReturnType<typeof show>, title: string) {
  return view.getByRole('heading', { name: new RegExp(`^${title}`) }).closest('li')?.dataset.status;
}

beforeEach(() => vi.clearAllMocks());

describe('ARCASetupGuide states', () => {
  it('starts on WSASS and the request, with the account CUIT shown read-only', () => {
    const view = show(EMPTY);

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

  it.each([null, '30-71234567-9'])('blocks the request without a valid CUIT (%s)', (cuit) => {
    const view = show(EMPTY, cuit);

    expect(view.getByText(messages.common.setup.ACCOUNT_TAX_ID.title)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.steps.generate.submit })).toBeNull();
  });

  it('generates the request without sending a CUIT', async () => {
    vi.mocked(createARCASetup).mockResolvedValue({ ok: true });
    const view = show(EMPTY);

    fireEvent.click(view.getByRole('button', { name: copy.steps.generate.submit }));

    await waitFor(() => expect(createARCASetup).toHaveBeenCalledWith());
    expect(refresh).toHaveBeenCalled();
  });

  it('moves to the steps done in ARCA once the request exists, the request ready to copy', () => {
    const view = show(REQUESTED);

    expect(stepState(view, copy.steps.generate.title)).toBe('done');
    for (const step of ['copy', 'certificate', 'authorize', 'upload'] as const) {
      expect(stepState(view, copy.steps[step].title)).toBe('current');
    }
    expect(view.getByLabelText(copy.steps.copy.label).textContent).toBe(REQUESTED.csr);
    expect(view.getByRole('button', { name: copy.steps.copy.copy })).toBeTruthy();
    expect(view.getByRole('button', { name: copy.steps.copy.download })).toBeTruthy();
  });

  it('shows what a whole certificate looks like instead of describing its delimiters', () => {
    const view = show(REQUESTED);
    expect(view.getByText(copy.pemCaption)).toBeTruthy();
    expect(view.container.textContent).toContain('-----BEGIN CERTIFICATE-----');
    expect(JSON.stringify(copy)).not.toMatch(/BEGIN|END CERTIFICATE/);
  });

  it('leaves the points of sale while a branch has none, after the certificate is in', () => {
    const view = show(INSTALLED);

    expect(stepState(view, copy.steps.upload.title)).toBe('done');
    expect(stepState(view, copy.steps.pointsOfSale.title)).toBe('current');
    expect(view.getByText('1 de 2 sucursales abiertas tienen punto de venta.')).toBeTruthy();
    expect(
      view.getByRole('link', { name: copy.steps.pointsOfSale.link }).getAttribute('href'),
    ).toBe(ROUTES.invoicingSection('points-of-sale'));
    expect(view.getByRole('button', { name: copy.steps.upload.download })).toBeTruthy();
  });

  it('says the certificate expired when it has', () => {
    const view = show({ ...INSTALLED, certificateExpiresAt: '2026-10-01T00:00:00Z' });
    expect(view.getByText(/El certificado venció el/)).toBeTruthy();
  });

  it('explains a server that cannot connect, with no steps to take', () => {
    const view = show({ ...EMPTY, enabled: false });
    expect(view.getByText(copy.disabled)).toBeTruthy();
    expect(view.getByText(copy.disabled).closest('[role="status"]')).toBeNull();
    expect(view.queryByRole('list')).toBeNull();
  });
});

describe('ARCASetupGuide certificate', () => {
  it('sends a pasted certificate trimmed, with Windows line endings normalized', async () => {
    vi.mocked(uploadARCACertificate).mockResolvedValue({ ok: true });
    const view = show(REQUESTED);

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: `\r\n  ${CERTIFICATE.replace(/\n/g, '\r\n')}  \r\n` },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(uploadARCACertificate).toHaveBeenCalledWith(CERTIFICATE));
    expect(toast.success).toHaveBeenCalledWith(copy.steps.upload.saved);
  });

  it('refuses text that is not one certificate block, without calling the API', async () => {
    const view = show(REQUESTED);
    const field = view.getByLabelText(copy.steps.upload.label, { exact: false });

    fireEvent.change(field, { target: { value: 'MIIDazCCAlOgAwIBAgIU' } });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(view.getByText(copy.steps.upload.invalid)).toBeTruthy());
    expect(uploadARCACertificate).not.toHaveBeenCalled();
  });

  it('reads a chosen file into the same field', async () => {
    const view = show(REQUESTED);
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');
    const file = new File([CERTIFICATE], 'certificado.pem');
    Object.defineProperty(file, 'text', { value: async () => CERTIFICATE });

    fireEvent.change(input!, { target: { files: [file] } });

    await waitFor(() =>
      expect(
        (view.getByLabelText(copy.steps.upload.label, { exact: false }) as HTMLTextAreaElement)
          .value,
      ).toBe(CERTIFICATE),
    );
  });

  it('puts the API’s refusal on the certificate field', async () => {
    vi.mocked(uploadARCACertificate).mockResolvedValue({ error: 'INVALID_INPUT' });
    const view = show(REQUESTED);

    fireEvent.change(view.getByLabelText(copy.steps.upload.label, { exact: false }), {
      target: { value: CERTIFICATE },
    });
    fireEvent.click(view.getByRole('button', { name: copy.steps.upload.submit }));

    await waitFor(() => expect(view.getByText(copy.errors.INVALID_INPUT)).toBeTruthy());
  });
});

describe('ARCASetupGuide disconnect', () => {
  it('asks first, then removes the connection', async () => {
    vi.mocked(disconnectARCA).mockResolvedValue({ ok: true });
    const view = show(INSTALLED);

    fireEvent.click(view.getByRole('button', { name: copy.disconnect }));
    const dialog = await view.findByRole('dialog');
    expect(within(dialog).getByText(/30-71234567-1/)).toBeTruthy();
    expect(disconnectARCA).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: copy.disconnect }));

    await waitFor(() => expect(disconnectARCA).toHaveBeenCalledOnce());
    expect(toast.success).toHaveBeenCalledWith(copy.disconnected);
  });

  it('offers no disconnect before there is anything to remove', () => {
    const view = show(EMPTY);
    expect(view.queryByRole('button', { name: copy.disconnect })).toBeNull();
  });
});
