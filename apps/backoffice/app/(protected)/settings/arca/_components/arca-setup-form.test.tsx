import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ARCASetupForm } from '@/app/(protected)/settings/arca/_components/arca-setup-form';
import { createARCASetup } from '@/app/(protected)/settings/arca/actions';
import type { ARCASetup } from '@/lib/api/arca-setup';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/(protected)/settings/arca/actions', () => ({
  createARCASetup: vi.fn(),
  uploadARCACertificate: vi.fn(),
  disconnectARCA: vi.fn(),
}));

const empty: ARCASetup = {
  enabled: true,
  taxId: '',
  csr: '',
  hasCertificate: false,
  certificateExpiresAt: null,
  pointOfSale: 0,
  verifiedAt: null,
};

function show(setup: ARCASetup = empty) {
  return render(
    <NextIntlClientProvider
      locale="es-AR"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ARCASetupForm setup={setup} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.resetAllMocks());

describe('ARCA onboarding', () => {
  it('guides the user through setup without asking for a fiscal password', () => {
    const view = show();
    expect(view.getByText(messages.arca.tutorial.title)).toBeTruthy();
    expect(view.getAllByRole('listitem')).toHaveLength(7);
    expect(view.container.querySelector('input[type="password"]')).toBeNull();
    expect(view.queryByRole('button', { name: messages.arca.verify })).toBeNull();
  });
  it('creates only a certificate request from the entered CUIT', async () => {
    vi.mocked(createARCASetup).mockResolvedValue({ ok: true });
    const view = show();
    fireEvent.change(view.getByRole('textbox', { name: new RegExp(messages.arca.taxId.label) }), {
      target: { value: '20-32964233-0' },
    });
    const form = view.container.querySelector('form');
    if (!form) throw new Error('no identity form');
    fireEvent.submit(form);
    await waitFor(() => expect(createARCASetup).toHaveBeenCalledWith({ taxId: '20-32964233-0' }));
  });
  it('routes branch configuration to branch settings without an editable point of sale', () => {
    const view = show({ ...empty, taxId: '20329642330', csr: 'request', hasCertificate: true });
    expect(
      view.getByRole('link', { name: messages.arca.pointOfSale.configure }).getAttribute('href'),
    ).toBe('/settings/branches');
    expect(view.queryByRole('textbox', { name: messages.arca.pointOfSale.label })).toBeNull();
  });
});
