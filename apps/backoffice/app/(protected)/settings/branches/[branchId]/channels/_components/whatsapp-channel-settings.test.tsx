import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhatsAppChannelSettings } from '@/app/(protected)/settings/branches/[branchId]/channels/_components/whatsapp-channel-settings';
import { saveWhatsAppChannel } from '@/app/(protected)/settings/branches/[branchId]/channels/actions';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/branches/[branchId]/channels/actions', () => ({
  saveWhatsAppChannel: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const CHANNEL = {
  id: 'c0000000-0000-4000-8000-000000000001',
  branchId: 'b0000000-0000-4000-8000-000000000002',
  type: 'WHATSAPP' as const,
  identifier: '+15556530731',
  isActive: true,
  isConfigured: true,
};
const copy = messages.branchChannels;

beforeEach(() => vi.resetAllMocks());

function renderSettings(configured = true) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <WhatsAppChannelSettings
        branchName="Moron"
        channel={{ ...CHANNEL, isConfigured: configured }}
      />
    </NextIntlClientProvider>,
  );
}

async function fillCredentials() {
  fireEvent.click(screen.getByRole('button', { name: copy.update }));
  await screen.findByRole('dialog');
  const values = {
    phoneNumberId: '1379080161952101',
    businessAccountId: '3580903055408468',
    accessToken: 'test-access-token',
    webhookVerifyToken: 'test-verification-token',
  };
  for (const name of Object.keys(values) as Array<keyof typeof values>) {
    fireEvent.change(screen.getByLabelText(copy[name].label, { exact: false }), {
      target: { value: values[name] },
    });
  }
  return values;
}

describe('WhatsAppChannelSettings', () => {
  it('shows the configured number and opens with empty, masked credential fields', async () => {
    renderSettings();
    expect(screen.getByText(CHANNEL.identifier)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: copy.update }));
    const token = await screen.findByLabelText(copy.accessToken.label, { exact: false });
    expect(token).toHaveProperty('type', 'password');
    expect(token).toHaveProperty('value', '');
    expect(token).toHaveProperty('maxLength', 4096);
    expect(screen.getByLabelText(copy.webhookVerifyToken.label, { exact: false })).toHaveProperty(
      'value',
      '',
    );
  });

  it('does not submit an empty replacement to the server', async () => {
    const view = renderSettings(false);
    fireEvent.click(screen.getByRole('button', { name: copy.configure }));
    await screen.findByRole('dialog');
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() => expect(screen.getByText(copy.accessToken.required)).toBeTruthy());
    expect(saveWhatsAppChannel).not.toHaveBeenCalled();
  });

  it('updates just the access token and leaves the webhook verification token empty', async () => {
    vi.mocked(saveWhatsAppChannel).mockResolvedValue({ ok: true });
    const view = renderSettings();
    fireEvent.click(screen.getByRole('button', { name: copy.update }));
    await screen.findByRole('dialog');
    fireEvent.change(screen.getByLabelText(copy.accessToken.label, { exact: false }), {
      target: { value: 'new-access-token' },
    });
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() =>
      expect(saveWhatsAppChannel).toHaveBeenCalledWith(CHANNEL.branchId, CHANNEL.id, {
        identifier: CHANNEL.identifier,
        phoneNumberId: '',
        businessAccountId: '',
        accessToken: 'new-access-token',
        webhookVerifyToken: '',
        isActive: true,
      }),
    );
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('saves the complete form with the displayed branch and channel, then closes', async () => {
    vi.mocked(saveWhatsAppChannel).mockResolvedValue({ ok: true });
    const view = renderSettings();
    const values = await fillCredentials();
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() =>
      expect(saveWhatsAppChannel).toHaveBeenCalledWith(CHANNEL.branchId, CHANNEL.id, {
        ...values,
        identifier: CHANNEL.identifier,
        isActive: true,
      }),
    );
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('keeps the form open and explains an API refusal without discarding entered values', async () => {
    vi.mocked(saveWhatsAppChannel).mockResolvedValue({ error: 'CHANNEL_IDENTIFIER' });
    const view = renderSettings();
    await fillCredentials();
    fireEvent.submit(view.baseElement.querySelector('form')!);
    expect(await screen.findByText(copy.errors.CHANNEL_IDENTIFIER)).toBeTruthy();
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(screen.getByLabelText(copy.accessToken.label, { exact: false })).toHaveProperty(
      'value',
      'test-access-token',
    );
  });

  it('disables credential editing and prevents closing while saving', async () => {
    let finishSave!: (result: { ok: true }) => void;
    vi.mocked(saveWhatsAppChannel).mockImplementation(
      () =>
        new Promise((resolve) => {
          finishSave = resolve;
        }),
    );
    const view = renderSettings();
    await fillCredentials();
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() =>
      expect(screen.getByLabelText(copy.accessToken.label, { exact: false })).toHaveProperty(
        'disabled',
        true,
      ),
    );
    expect(screen.getByRole('button', { name: copy.cancel })).toHaveProperty('disabled', true);
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(screen.getByRole('dialog')).toBeTruthy();
    finishSave({ ok: true });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});
