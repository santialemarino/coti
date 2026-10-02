import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { SetupNotice } from '@/components/setup-notice';
import { ROUTES } from '@/config/routes';
import type { SetupIssue } from '@/lib/utils/setup-issues';
import messages from '@/translations/es.json';

function renderNotice(issue: SetupIssue, isAdmin: boolean) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <SetupNotice issue={issue} isAdmin={isAdmin} />
    </NextIntlClientProvider>,
  );
}

describe('SetupNotice', () => {
  it('sends an administrator to the screen that fixes it', () => {
    renderNotice('BRANCH_EMAIL', true);

    const fix = screen.getByRole('link', { name: 'Configurar correo' });
    expect(fix.getAttribute('href')).toBe(ROUTES.branchSettings);
    expect(screen.queryByText(/Pedile a un administrador/)).toBeNull();
  });

  // A seller cannot reach the fix, so a link would be a dead end; naming who can is the action.
  it('tells anyone else who can fix it, with no link', () => {
    renderNotice('BRANCH_EMAIL', false);

    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText(/Pedile a un administrador que lo configure\./)).toBeTruthy();
  });

  it('offers no fix for an issue no screen can fix', () => {
    renderNotice('NO_INTAKE_CHANNELS', true);

    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('Esta sucursal no tiene canales abiertos')).toBeTruthy();
  });
});
