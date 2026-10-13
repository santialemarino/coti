import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import {
  ARCAConnectionCard,
  arcaConnectionState,
} from '@/app/(protected)/settings/invoicing/_components/arca-connection-card';
import { ROUTES } from '@/config/routes';
import type { ARCASetup } from '@/lib/api/arca-setup';
import messages from '@/translations/es.json';

const NOW = '2026-10-13T12:00:00.000Z';
const NONE: ARCASetup = {
  enabled: true,
  taxId: '',
  csr: '',
  hasCertificate: false,
  certificate: '',
  certificateExpiresAt: null,
};
const CONNECTED: ARCASetup = {
  ...NONE,
  taxId: '30712345679',
  csr: 'request',
  hasCertificate: true,
  certificate: 'certificate',
  certificateExpiresAt: '2028-10-13T00:00:00Z',
};
const copy = messages.invoicing.settings.connection;

function card(setup: ARCASetup) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ARCAConnectionCard setup={setup} now={NOW} />
    </NextIntlClientProvider>,
  );
}

describe('arcaConnectionState', () => {
  it.each([
    [{ ...NONE, enabled: false }, 'disabled'],
    [NONE, 'notConnected'],
    [{ ...NONE, csr: 'request' }, 'inProgress'],
    [CONNECTED, 'connected'],
    [{ ...CONNECTED, certificateExpiresAt: '2026-10-01T00:00:00Z' }, 'expired'],
  ] as const)('reads %o as %s', (setup, state) => {
    expect(arcaConnectionState(setup, NOW)).toBe(state);
  });
});

describe('ARCAConnectionCard', () => {
  it('shows the CUIT and the certificate expiry once connected', () => {
    const view = card(CONNECTED);
    expect(view.getByText(copy.states.connected)).toBeTruthy();
    expect(view.getByText('30-71234567-9')).toBeTruthy();
    expect(view.getByText(copy.expires)).toBeTruthy();
    expect(view.getByRole('link', { name: copy.actions.manage }).getAttribute('href')).toBe(
      ROUTES.arcaSettings,
    );
  });

  it('leads to the guided setup before there is a connection', () => {
    const view = card(NONE);
    expect(view.getByRole('link', { name: copy.actions.start }).getAttribute('href')).toBe(
      ROUTES.arcaSettings,
    );
    expect(view.queryByText(copy.cuit)).toBeNull();
  });

  it('offers no action when the server cannot connect', () => {
    const view = card({ ...NONE, enabled: false });
    expect(view.getByText(copy.bodies.disabled)).toBeTruthy();
    expect(view.queryByRole('link')).toBeNull();
  });
});
