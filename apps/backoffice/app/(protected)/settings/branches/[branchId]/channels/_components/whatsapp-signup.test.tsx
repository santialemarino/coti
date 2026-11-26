import { useEffect } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it, vi } from 'vitest';

import { connectWhatsAppSignup } from '@/app/(protected)/settings/branches/[branchId]/channels/actions';
import messages from '@/translations/es.json';
import { WhatsAppSignup } from './whatsapp-signup';

const { refresh } = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh }) }));
vi.mock('next/script', () => ({
  default: function MockScript({ onReady }: { onReady: () => void }) {
    useEffect(() => {
      onReady();
    }, [onReady]);
    return null;
  },
}));
vi.mock('@/app/(protected)/settings/branches/[branchId]/channels/actions', () => ({
  connectWhatsAppSignup: vi.fn(),
}));

it('uses the selected channel and rejects events from outside Meta', async () => {
  const branch = 'b0000000-0000-4000-8000-000000000002';
  const channel = 'c0000000-0000-4000-8000-000000000001';
  let finish: (response: { authResponse: { code: string } }) => void = () => {};
  vi.stubGlobal('FB', {
    init: vi.fn(),
    login: (callback: typeof finish) => {
      finish = callback;
    },
  });
  vi.mocked(connectWhatsAppSignup).mockResolvedValue({ ok: true });
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <WhatsAppSignup
        channel={{
          id: channel,
          branchId: branch,
          type: 'WHATSAPP',
          identifier: null,
          isActive: true,
          isConfigured: false,
        }}
        settings={{ appId: '111', configurationId: '222', graphVersion: 'v25.0', received: false }}
      />
    </NextIntlClientProvider>,
  );
  try {
    fireEvent.click(screen.getByRole('button', { name: messages.branchChannels.signup.connect }));
    act(() => finish({ authResponse: { code: 'exchangeable' } }));
    const data = JSON.stringify({
      type: 'WA_EMBEDDED_SIGNUP',
      event: 'FINISH',
      data: { phone_number_id: '123', waba_id: '456' },
    });
    act(() =>
      window.dispatchEvent(
        new MessageEvent('message', { origin: 'https://evilfacebook.com', data }),
      ),
    );
    expect(connectWhatsAppSignup).not.toHaveBeenCalled();
    act(() =>
      window.dispatchEvent(
        new MessageEvent('message', { origin: 'https://www.facebook.com', data }),
      ),
    );
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(connectWhatsAppSignup).toHaveBeenCalledExactlyOnceWith(branch, channel, {
      code: 'exchangeable',
      phoneNumberId: '123',
      businessAccountId: '456',
    });
  } finally {
    view.unmount();
    vi.unstubAllGlobals();
  }
});
