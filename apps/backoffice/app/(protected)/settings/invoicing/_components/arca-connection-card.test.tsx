import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { ARCAConnectionCard } from '@/app/(protected)/settings/invoicing/_components/arca-connection-card';
import { ROUTES } from '@/config/routes';
import type { ARCASetup } from '@/lib/api/arca-setup';
import type { ARCAConnectionState } from '@/lib/utils/arca-connection';
import messages from '@/translations/es.json';

const NONE: ARCASetup = {
  enabled: true,
  taxId: '',
  csr: '',
  hasCertificate: false,
  certificate: '',
  certificateExpiresAt: null,
};
const INSTALLED: ARCASetup = {
  ...NONE,
  taxId: '30712345671',
  csr: 'request',
  hasCertificate: true,
  certificate: 'certificate',
  certificateExpiresAt: '2028-10-13T12:00:00Z',
};
const copy = messages.invoicing.settings.connection;

function card(setup: ARCASetup, state: ARCAConnectionState) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ARCAConnectionCard setup={setup} state={state} />
    </NextIntlClientProvider>,
  );
}

describe('ARCAConnectionCard', () => {
  it.each([
    ['notConnected', NONE, copy.actions.start, ROUTES.arcaSettings],
    ['inProgress', { ...NONE, csr: 'request' }, copy.actions.continue, ROUTES.arcaSettings],
    ['pointsMissing', INSTALLED, copy.actions.continue, ROUTES.arcaStep('points-of-sale')],
    ['expiringSoon', INSTALLED, copy.actions.manage, ROUTES.arcaSettings],
    ['expired', INSTALLED, copy.actions.manage, ROUTES.arcaSettings],
    ['connected', INSTALLED, copy.actions.manage, ROUTES.arcaSettings],
  ] as const)('names %s and leads to the right place', (state, setup, action, href) => {
    const view = card(setup, state);
    expect(view.getByText(copy.states[state])).toBeTruthy();
    expect(view.getByRole('link', { name: action }).getAttribute('href')).toBe(href);
  });

  it.each(['pointsMissing', 'expiringSoon', 'expired', 'connected'] as const)(
    'shows the CUIT and the expiry once a certificate is in (%s)',
    (state) => {
      const view = card(INSTALLED, state);
      expect(view.getByText('30-71234567-1')).toBeTruthy();
      expect(view.getByText(copy.expires)).toBeTruthy();
    },
  );

  it('names the expiry date when the certificate is about to expire', () => {
    const view = card(INSTALLED, 'expiringSoon');
    expect(view.getByText(/El certificado vence el .*2028/)).toBeTruthy();
  });

  it('shows no CUIT before a certificate exists', () => {
    expect(card(NONE, 'notConnected').queryByText(copy.cuit)).toBeNull();
  });

  it('offers no action when the server cannot connect', () => {
    const view = card({ ...NONE, enabled: false }, 'disabled');
    expect(view.getByText(copy.bodies.disabled)).toBeTruthy();
    expect(view.queryByRole('link')).toBeNull();
  });
});
